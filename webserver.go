package main

import (
	"net/http"
	"strconv"

	"golang.org/x/net/websocket"

	log "github.com/Sirupsen/logrus"
	"github.com/gin-gonic/gin"
)

func webserver(port int) {
	r := gin.Default()

	api := r.Group("/api")
	api.GET("/zone/1/config", func(c *gin.Context) {
		cfg, ok := getConfig()
		if ok {
			c.JSON(200, cfg)
		}
	})

	api.PUT("/zone/1/config", func(c *gin.Context) {
		var args TStatZoneConfig

		if c.Bind(&args) == nil {
			params := TStatZoneParams{}
			flags := byte(0)

			if len(args.FanMode) > 0 {
				params.Z1FanMode = stringFanModeToRaw(args.FanMode)
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
				infinity.Write(devTSTAT, tTSTAT_ZONE_PARAMS, []byte{0x00, 0x00, flags}, params)
			}

			if len(args.Mode) > 0 {
				p := TStatCurrentParams{Mode: stringModeToRaw(args.Mode)}
				infinity.Write(devTSTAT, tTSTAT_CURRENT_PARAMS, []byte{0x00, 0x00, 0x10}, p)
			}
		} else {
			log.Printf("bind failed")
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
