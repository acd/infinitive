package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"golang.org/x/net/websocket"

	"github.com/acd/infinitive/internal/assets"
	"github.com/acd/infinitive/internal/cache"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func handleErrors(c *gin.Context) {
	c.Next()

	if len(c.Errors) > 0 {
		c.JSON(-1, c.Errors) // -1 == not override the current error code
	}
}

type webserver struct {
	srv   *http.Server
	cache *cache.Cache
}

func launchWebserver(port int, cache *cache.Cache) error {
	ws := webserver{
		cache: cache,
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
		tss, ok := getTstatSettings()
		if ok {
			c.JSON(200, tss)
		}
	})

	api.GET("/zone/1/config", func(c *gin.Context) {
		cfg, ok := getConfig()
		if ok {
			c.JSON(200, cfg)
		}
	})

	api.GET("/zone/1/airhandler", func(c *gin.Context) {
		ah, ok := getAirHandler(ws.cache)
		if ok {
			c.JSON(200, ah)
		}
	})

	api.GET("/zone/1/heatpump", func(c *gin.Context) {
		hp, ok := getHeatPump(ws.cache)
		if ok {
			c.JSON(200, hp)
		}
	})

	api.GET("/zone/1/vacation", func(c *gin.Context) {
		vac := TStatVacationParams{}
		ok := infinity.ReadTable(devTSTAT, &vac)
		if ok {
			c.JSON(200, vac.toAPI())
		}
	})

	api.PUT("/zone/1/vacation", func(c *gin.Context) {
		var args APIVacationConfig

		if c.Bind(&args) != nil {
			log.Printf("bind failed")
			return
		}

		params := TStatVacationParams{}
		flags := params.fromAPI(&args)

		infinity.WriteTable(devTSTAT, params, flags)
	})

	api.PUT("/zone/:zone/config", func(c *gin.Context) {
		var args TStatZoneConfig

		if c.Bind(&args) != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}

		zone, err := strconv.Atoi(c.Param("zone"))
		if err != nil || zone < 1 || zone > 8 {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid zone: %d", zone))
			return
		}

		params := TStatZoneParams{}
		flags := byte(0)

		if len(args.FanMode) > 0 {
			mode, _ := stringFanModeToRaw(args.FanMode)
			// FIXME: check for ok here
			params.setZonalField(zone, "FanMode", mode)
			flags |= 0x01
		}

		if args.Hold != nil {
			if *args.Hold {
				params.ZoneHold = 0x01
			} else {
				params.ZoneHold = 0x00
			}
			flags |= 0x02
		}

		if args.HeatSetpoint > 0 {
			params.setZonalField(zone, "HeatSetpoint", args.HeatSetpoint)
			flags |= 0x04
		}

		if args.CoolSetpoint > 0 {
			params.setZonalField(zone, "CoolSetpoint", args.CoolSetpoint)
			flags |= 0x08
		}

		if flags != 0 {
			log.Printf("calling doWrite with flags: %x", flags)
			infinity.WriteTable(devTSTAT, params, flags)
		}

		if len(args.Mode) > 0 {
			p := TStatCurrentParams{Mode: stringModeToRaw(args.Mode)}
			infinity.WriteTable(devTSTAT, p, 0x10)
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
		var addr InfinityTableAddr
		copy(addr[:], a[0:3])
		raw := InfinityProtocolRawRequest{&[]byte{}}

		success := infinity.Read(uint16(d), addr, raw)

		if success {
			c.JSON(200, gin.H{"response": hex.EncodeToString(*raw.data)})
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

func (ws *webserver) websocketListener(wsConn *websocket.Conn) {
	listener := &EventListener{make(chan []byte, 32)}

	defer func() {
		Dispatcher.deregister <- listener
		log.Printf("closing websocket")
		err := wsConn.Close()
		if err != nil {
			log.Println("error on ws close:", err.Error())
		}
	}()

	Dispatcher.register <- listener

	log.Printf("dumping cached data")
	for source, data := range ws.cache.Dump() {
		log.Printf("dumping %s", source)
		wsConn.Write(serializeEvent(source, data))
	}

	// wait for events
	for message := range listener.ch {
		if _, err := wsConn.Write(message); err != nil {
			log.Printf("error on websocket write: %s", err.Error())
			return
		}
	}
}
