package main

import (
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"golang.org/x/net/websocket"

	log "github.com/sirupsen/logrus"
	"github.com/gin-gonic/gin"
)

func handleErrors(c *gin.Context) {
	c.Next()

	if len(c.Errors) > 0 {
		c.JSON(-1, c.Errors) // -1 == not override the current error code
	}
}

func webserver(port int) {
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
		ah, ok := getAirHandler()
		if ok {
			c.JSON(200, ah)
		}
	})

	api.GET("/zone/1/heatpump", func(c *gin.Context) {
		hp, ok := getHeatPump()
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

	api.PUT("/zone/1/config", func(c *gin.Context) {
		var args TStatZoneConfig

		if c.Bind(&args) == nil {
			params := TStatZoneParams{}
			flags := byte(0)

			if len(args.FanMode) > 0 {
				mode, _ := stringFanModeToRaw(args.FanMode)
				// FIXME: check for ok here
				params.Z1FanMode = mode
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
				params.Z1HeatSetpoint = args.HeatSetpoint
				flags |= 0x04
			}

			if args.CoolSetpoint > 0 {
				params.Z1CoolSetpoint = args.CoolSetpoint
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
		} else {
			log.Printf("bind failed")
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
		h := websocket.Handler(attachListener)
		h.ServeHTTP(c.Writer, c.Request)
	})

	r.StaticFS("/ui", assetFS())
	// r.Static("/ui", "github.com/acd/infinitease/assets")

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "ui")
	})

	r.Run(":" + strconv.Itoa(port)) // listen and server on 0.0.0.0:8080
}

func attachListener(ws *websocket.Conn) {
	listener := &EventListener{make(chan []byte, 32)}

	defer func() {
		Dispatcher.deregister <- listener
		log.Printf("closing websocket")
		err := ws.Close()
		if err != nil {
			log.Println("error on ws close:", err.Error())
		}
	}()

	Dispatcher.register <- listener

	log.Printf("dumping cached data")
	for source, data := range cache {
		log.Printf("dumping %s", source)
		ws.Write(serializeEvent(source, data))
	}

	// wait for events
	for {
		select {
		case message, ok := <-listener.ch:
			if !ok {
				log.Printf("read from listener.ch was not okay")
				return
			} else {
				_, err := ws.Write(message)
				if err != nil {
					log.Printf("error on websocket write: %s", err.Error())
					return
				}
			}
		}
	}
}
