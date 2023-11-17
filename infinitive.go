package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/acd/infinitive/internal/cache"
	"github.com/acd/infinitive/internal/dispatcher"
	log "github.com/sirupsen/logrus"
)

const (
	blowerCacheKey   = "blower"
	heatpumpCacheKey = "heatpump"
	tstatCacheKey    = "tstat"
)

type TStatZoneConfig struct {
	TempUnit        string `json:"tempUnit"`
	CurrentTemp     uint8  `json:"currentTemp"`
	CurrentHumidity uint8  `json:"currentHumidity"`
	OutdoorTemp     int8   `json:"outdoorTemp"`
	Mode            string `json:"mode"`
	Stage           uint8  `json:"stage"`
	FanMode         string `json:"fanMode"`
	Hold            *bool  `json:"hold"`
	HeatSetpoint    uint8  `json:"heatSetpoint"`
	CoolSetpoint    uint8  `json:"coolSetpoint"`
	RawMode         uint8  `json:"rawMode"`
}

type AirHandler struct {
	BlowerRPM  uint16 `json:"blowerRPM"`
	AirFlowCFM uint16 `json:"airFlowCFM"`
	ElecHeat   bool   `json:"elecHeat"`
}

type HeatPump struct {
	TempUnit    string  `json:"tempUnit"`
	CoilTemp    float32 `json:"coilTemp"`
	OutsideTemp float32 `json:"outsideTemp"`
	Stage       uint8   `json:"stage"`
}

var infinity *InfinityProtocol

func getConfig(zone int) (*TStatZoneConfig, bool) {
	cfg := TStatZoneParams{}
	ok := infinity.ReadTable(devTSTAT, &cfg)
	if !ok {
		return nil, false
	}

	params := TStatCurrentParams{}
	ok = infinity.ReadTable(devTSTAT, &params)
	if !ok {
		return nil, false
	}

	hold := new(bool)
	*hold = cfg.ZoneHold&(1<<zone-1) != 0

	return &TStatZoneConfig{
		CurrentTemp:     params.getZonalField(zone, "CurrentTemp").(uint8),
		CurrentHumidity: params.getZonalField(zone, "CurrentHumidity").(uint8),
		OutdoorTemp:     params.OutdoorAirTemp,
		Mode:            rawModeToString(params.Mode & 0xf),
		Stage:           params.Mode >> 5,
		FanMode:         rawFanModeToString(cfg.getZonalField(zone, "FanMode").(uint8)),
		Hold:            hold,
		HeatSetpoint:    cfg.getZonalField(zone, "HeatSetpoint").(uint8),
		CoolSetpoint:    cfg.getZonalField(zone, "CoolSetpoint").(uint8),
		RawMode:         params.Mode,
	}, true
}

func getTstatSettings() (*TStatSettings, bool) {
	tss := TStatSettings{}
	if !infinity.ReadTable(devTSTAT, &tss) {
		return nil, false
	}
	return &tss, true
}

func getAirHandler(cache *cache.Cache) (AirHandler, bool) {
	b := cache.Get(blowerCacheKey)
	tb, ok := b.(*AirHandler)
	if !ok {
		return AirHandler{}, false
	}
	return *tb, true
}

func getHeatPump(cache *cache.Cache) (HeatPump, bool) {
	h := cache.Get(heatpumpCacheKey)
	th, ok := h.(*HeatPump)
	if !ok {
		return HeatPump{}, false
	}
	return *th, true
}

func statePoller(cache *cache.Cache) {
	ticker := time.NewTicker(time.Second)

	for {
		if c, ok := getConfig(1); ok {
			cache.Update(tstatCacheKey, c)
		}

		<-ticker.C
	}
}

func attachSnoops(cache *cache.Cache) {
	// Snoop Heat Pump responses
	infinity.snoopResponse(0x5000, 0x51ff, func(frame InfinityFrame) {
		if heatPump, ok := getHeatPump(cache); ok {
			data := frame.data[3:]
			if bytes.Equal(frame.data[0:3], []byte{0x00, 0x3e, 0x01}) {
				heatPump.CoilTemp = float32(binary.BigEndian.Uint16(data[2:4])) / float32(16)
				heatPump.OutsideTemp = float32(binary.BigEndian.Uint16(data[0:2])) / float32(16)
				log.Debugf("heat pump coil temp is: %f", heatPump.CoilTemp)
				log.Debugf("heat pump outside temp is: %f", heatPump.OutsideTemp)
			} else if bytes.Equal(frame.data[0:3], []byte{0x00, 0x3e, 0x02}) {
				heatPump.Stage = data[0] >> 1
				log.Debugf("HP stage is: %d", heatPump.Stage)
			}
			cache.Update(heatpumpCacheKey, &heatPump)
		}
	})

	// Snoop Air Handler responses
	infinity.snoopResponse(0x4000, 0x42ff, func(frame InfinityFrame) {
		if airHandler, ok := getAirHandler(cache); ok {
			data := frame.data[3:]
			if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x06}) {
				airHandler.BlowerRPM = binary.BigEndian.Uint16(data[1:5])
				log.Debugf("blower RPM is: %d", airHandler.BlowerRPM)
			} else if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x16}) {
				airHandler.AirFlowCFM = binary.BigEndian.Uint16(data[4:8])
				airHandler.ElecHeat = data[0]&0x03 != 0
				log.Debugf("air flow CFM is: %d", airHandler.AirFlowCFM)
			}
			cache.Update(blowerCacheKey, &airHandler)
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

	dispatcher := dispatcher.New()
	cache := cache.New(dispatcher.BroadcastEvent)
	// Set default values for structs the UI cares about
	cache.Update(blowerCacheKey, &AirHandler{})
	cache.Update(heatpumpCacheKey, &HeatPump{})

	attachSnoops(cache)
	err := infinity.Open()
	if err != nil {
		log.Panicf("error opening serial port: %s", err.Error())
	}

	go statePoller(cache)
	launchWebserver(*httpPort, cache, dispatcher)
}
