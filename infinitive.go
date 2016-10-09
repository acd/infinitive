package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

        log "github.com/Sirupsen/logrus"
)

type TStatZoneConfig struct {
	CurrentTemp     uint8  `json:"currentTemp"`
	CurrentHumidity uint8  `json:"currentHumidity"`
	OutdoorTemp     uint8  `json:"outdoorTemp"`
	Mode            string `json:"mode"`
	CoolActive      bool   `json:"coolActive"`
	HeatActive      bool   `json:"heatActive"`
	FanMode         string `json:"fanMode"`
	Hold            *bool  `json:"hold"`
	HeatSetpoint    uint8  `json:"heatSetpoint"`
	CoolSetpoint    uint8  `json:"coolSetpoint"`
}

type AirHandlerBlower struct {
	BlowerRPM uint16 `json:"blowerRPM"`
}

type AirHandlerDuct struct {
	AirFlowCFM uint16 `json:"airFlowCFM"`
}

var infinity *InfinityProtocol

func getConfig() (*TStatZoneConfig, bool) {
	cfg := TStatZoneParams{}
	ok := infinity.Read(devTSTAT, tTSTAT_ZONE_PARAMS, &cfg)
	if !ok {
		return nil, false
	}

	params := TStatCurrentParams{}
	ok = infinity.Read(devTSTAT, tTSTAT_CURRENT_PARAMS, &params)
	if !ok {
		return nil, false
	}

	hold := new(bool)
	*hold = cfg.ZoneHold&0x01 == 1

	return &TStatZoneConfig{
		CurrentTemp:     params.Z1CurrentTemp,
		CurrentHumidity: params.Z1CurrentHumidity,
		OutdoorTemp:     params.OutdoorAirTemp,
		Mode:            rawModeToString(params.Mode & 0xf),
		/* I thought the active modes were bits in the mode field but that doesn't seem to be correct.
		      Those bits may indicate stage instead...
		   		CoolActive:      (params.Mode&0x40 != 0),
		   		HeatActive:      (params.Mode&0x20 != 0),
		*/
		FanMode:      rawFanModeToString(cfg.Z1FanMode),
		Hold:         hold,
		HeatSetpoint: cfg.Z1HeatSetpoint,
		CoolSetpoint: cfg.Z1CoolSetpoint,
	}, true
}

func statePoller() {
	for {
		c, ok := getConfig()
		if ok {
			cache.update("tstat", c)
		}

		time.Sleep(time.Second * 1)
	}
}

func attachSnoops() {
	// Snoop Heat Pump responses
	infinity.snoopResponse(0x5000, 0x51ff, func(frame *InfinityFrame) {
		data := frame.data[3:]

		if bytes.Equal(frame.data[0:3], []byte{0x00, 0x3e, 0x01}) {
			coilTemp := float32(binary.BigEndian.Uint16(data[2:4])) / float32(16)
			outsideTemp := float32(binary.BigEndian.Uint16(data[0:2])) / float32(16)

			log.Printf("heat pump coil temp is: %f", coilTemp)
			log.Printf("heat pump outside temp is: %f", outsideTemp)
		}

	})

	// Snoop Air Handler responses
	infinity.snoopResponse(0x4000, 0x41ff, func(frame *InfinityFrame) {
		data := frame.data[3:]

		if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x06}) {
			blowerRPM := binary.BigEndian.Uint16(data[1:5])
			log.Debugf("blower RPM is: %d", blowerRPM)
			cache.update("blower", &AirHandlerBlower{BlowerRPM: blowerRPM})
		} else if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x16}) {
			airFlowCFM := binary.BigEndian.Uint16(data[4:8])
			log.Debugf("air flow CFM is: %d", airFlowCFM)
			cache.update("duct", &AirHandlerDuct{AirFlowCFM: airFlowCFM})
		}
	})
}

func main() {
	httpPort := flag.Int("httpport", 8080, "HTTP port to listen on")
	serialPort := flag.String("serial", "", "path to serial port")

	flag.Parse()

	if len(*serialPort) == 0 {
		fmt.Print("must provide serial\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.SetLevel(log.DebugLevel)

	infinity = &InfinityProtocol{device: *serialPort}
	attachSnoops()
	err := infinity.Open()
	if err != nil {
		log.Panicf("error opening serial port: %s", err.Error())
	}

	go statePoller()
	webserver(*httpPort)
}
