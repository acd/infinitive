package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"golang.org/x/net/websocket"

	"github.com/acd/infinitive/infinity"
	"github.com/acd/infinitive/internal/assets"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func handleErrors(c *gin.Context) {
	c.Next()

	if len(c.Errors) > 0 {
		c.JSON(-1, c.Errors) // -1 == do not override the current error code
	}
}

type webserver struct {
	srv *http.Server
	api *infinity.Api
}

func launchWebserver(port int, api *infinity.Api) error {
	ws := webserver{
		api: api,
	}
	ws.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: ws.buildEngine().Handler(),
	}

	return ws.srv.ListenAndServe()
}

func (ws *webserver) buildEngine() *gin.Engine {
	r := gin.Default()
	r.Use(handleErrors) // attach error handling middleware

	api := r.Group("/api")

	api.GET("/tstat/settings", func(c *gin.Context) {
		tss, ok := ws.api.GetTstatSettings()
		if ok {
			c.JSON(200, tss)
		}
	})

	parseZone := func(c *gin.Context) (int, bool) {
		zone, err := strconv.Atoi(c.Param("zone"))
		if err != nil || zone < 1 || zone > 8 {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid zone: %s", c.Param("zone")))
			return 0, false
		}
		return zone, true
	}

	api.GET("/zone/:zone/config", func(c *gin.Context) {
		zone, ok := parseZone(c)
		if !ok {
			return
		}

		cfg, ok := ws.api.GetConfig(zone)
		if ok {
			c.JSON(200, cfg)
		}
	})

	getAirHandler := func(c *gin.Context) {
		ah, ok := ws.api.GetAirHandler()
		if ok {
			c.JSON(200, ah)
		}
	}

	getHeatPump := func(c *gin.Context) {
		hp, ok := ws.api.GetHeatPump()
		if ok {
			c.JSON(200, hp)
		}
	}

	api.GET("/airhandler", getAirHandler)
	api.GET("/heatpump", getHeatPump)
	// The routes below are for backward compatibility
	api.GET("/zone/1/airhandler", getAirHandler)
	api.GET("/zone/1/heatpump", getHeatPump)

	api.GET("/zone/1/vacation", func(c *gin.Context) {
		vac := infinity.TStatVacationParams{}
		ok := ws.api.Protocol.ReadTable(infinity.DevTSTAT, &vac)
		if ok {
			c.JSON(200, vac.ToAPI())
		}
	})

	api.PUT("/zone/1/vacation", func(c *gin.Context) {
		var args infinity.APIVacationConfig

		if c.Bind(&args) != nil {
			log.Printf("bind failed")
			return
		}

		params := infinity.TStatVacationParams{}
		flags := params.FromAPI(&args)

		ws.api.UpdateThermostat(params, flags)
	})

	api.PUT("/zone/:zone/config", func(c *gin.Context) {
		zone, ok := parseZone(c)
		if !ok {
			return
		}

		var args infinity.TStatZoneConfig
		if c.Bind(&args) != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}

		params := infinity.TStatZoneParams{}
		flags := byte(0)

		if len(args.FanMode) > 0 {
			mode, _ := infinity.StringFanModeToRaw(args.FanMode)
			// FIXME: check for ok here
			params.SetZonalField(zone, "FanMode", mode)
			flags |= 0x01
		}

		if args.Hold != nil {
			// We have to read the current settings since hold is a bitfield and we need to
			// retain the configuration for other zones.
			priorParams := infinity.TStatZoneParams{}
			ok := ws.api.Protocol.ReadTable(infinity.DevTSTAT, &priorParams)
			if !ok {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}

			params.ZoneHold = priorParams.ZoneHold
			if *args.Hold {
				params.ZoneHold |= 1 << (zone - 1)
			} else {
				params.ZoneHold &= ^(1 << (zone - 1))
			}
			flags |= 0x02
		}

		if args.HeatSetpoint > 0 {
			params.SetZonalField(zone, "HeatSetpoint", args.HeatSetpoint)
			flags |= 0x04
		}

		if args.CoolSetpoint > 0 {
			params.SetZonalField(zone, "CoolSetpoint", args.CoolSetpoint)
			flags |= 0x08
		}

		if flags != 0 {
			ws.api.UpdateThermostat(params, flags)
		}

		if len(args.Mode) > 0 {
			p := infinity.TStatCurrentParams{Mode: infinity.StringModeToRaw(args.Mode)}
			ws.api.UpdateThermostat(p, 0x10)
		}
	})

	api.GET("/raw/:device/:table", func(c *gin.Context) {
		matched, _ := regexp.MatchString("^[a-f0-9]{4}$", c.Param("device"))
		if !matched {
			c.AbortWithError(400, errors.New("name must be a 4 character hex string"))
			return
		}
		matched, _ = regexp.MatchString("^[a-f0-9]{6}$", c.Param("table"))
		if !matched {
			c.AbortWithError(400, errors.New("table must be a 6 character hex string"))
			return
		}

		d, _ := strconv.ParseUint(c.Param("device"), 16, 16)
		a, _ := hex.DecodeString(c.Param("table"))

		if response := ws.api.GetTableRaw(uint16(d), a); response != nil {
			c.JSON(200, gin.H{"response": hex.EncodeToString(response)})
		} else {
			c.AbortWithError(504, errors.New("timed out waiting for response"))
		}
	})

	api.GET("/ws", func(c *gin.Context) {
		h := websocket.Handler(ws.websocketListener)
		h.ServeHTTP(c.Writer, c.Request)
	})

	r.StaticFS("/ui", http.FS(assets.Assets))

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "ui")
	})

	return r
}

func serializeUpdate(source string, data any) []byte {
	msg, _ := json.Marshal(&struct {
		Source string      `json:"source"`
		Data   interface{} `json:"data"`
	}{source, data})
	return msg
}

func (ws *webserver) websocketListener(wsConn *websocket.Conn) {
	listener := ws.api.NewListener()

	defer func() {
		listener.Close()
		log.Infof("%s: closing websocket", wsConn.RemoteAddr())
		if err := wsConn.Close(); err != nil {
			log.Errorf("%s: error on closing wsConn: %v", wsConn.RemoteAddr(), err)
		}
	}()

	for source, data := range ws.api.Cache.Dump() {
		wsConn.Write(serializeUpdate(source, data))
	}

	// wait for events
	for message := range listener.Receive() {
		if _, err := wsConn.Write(serializeUpdate(message.Source, message.Data)); err != nil {
			log.Infof("%s: error writing to wsConn: %v", wsConn.RemoteAddr(), err)
			return
		}
	}
}
