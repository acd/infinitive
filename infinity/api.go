package infinity

import (
	"bytes"
	"context"
	"encoding/binary"
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

type Api struct {
	ctx        context.Context
	Protocol   *Protocol
	dispatcher *dispatcher.Dispatcher
	Cache      *cache.Cache
}

func NewApi(ctx context.Context, device string) (*Api, error) {
	protocol, err := NewProtocol(device)
	if err != nil {
		return nil, err
	}

	dispatcher := dispatcher.New()

	cache := cache.New(dispatcher.BroadcastEvent)
	// Set default values for structs the UI cares about
	cache.Update(blowerCacheKey, &AirHandler{})
	cache.Update(heatpumpCacheKey, &HeatPump{})

	api := &Api{
		ctx:        ctx,
		Protocol:   protocol,
		dispatcher: dispatcher,
		Cache:      cache,
	}
	api.attachSnoops()
	go api.poller()
	return api, nil
}

func (a *Api) attachSnoops() {
	// Snoop Heat Pump responses
	a.Protocol.SnoopResponse(0x5000, 0x51ff, func(frame Frame) {
		if heatPump, ok := a.GetHeatPump(); ok {
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
			a.Cache.Update(heatpumpCacheKey, &heatPump)
		}
	})

	// Snoop Air Handler responses
	a.Protocol.SnoopResponse(0x4000, 0x42ff, func(frame Frame) {
		if airHandler, ok := a.GetAirHandler(); ok {
			data := frame.data[3:]
			if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x06}) {
				airHandler.BlowerRPM = binary.BigEndian.Uint16(data[1:5])
				log.Debugf("blower RPM is: %d", airHandler.BlowerRPM)
			} else if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x16}) {
				airHandler.AirFlowCFM = binary.BigEndian.Uint16(data[4:8])
				airHandler.ElecHeat = data[0]&0x03 != 0
				log.Debugf("air flow CFM is: %d", airHandler.AirFlowCFM)
			}
			a.Cache.Update(blowerCacheKey, &airHandler)
		}
	})
}

func (a *Api) poller() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c, ok := a.GetConfig(1); ok {
				a.Cache.Update(tstatCacheKey, c)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

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

func (a *Api) GetConfig(zone int) (*TStatZoneConfig, bool) {
	cfg := TStatZoneParams{}
	ok := a.Protocol.ReadTable(DevTSTAT, &cfg)
	if !ok {
		return nil, false
	}

	params := TStatCurrentParams{}
	ok = a.Protocol.ReadTable(DevTSTAT, &params)
	if !ok {
		return nil, false
	}

	hold := new(bool)
	*hold = cfg.ZoneHold&(1<<zone-1) != 0

	return &TStatZoneConfig{
		CurrentTemp:     params.GetZonalField(zone, "CurrentTemp").(uint8),
		CurrentHumidity: params.GetZonalField(zone, "CurrentHumidity").(uint8),
		OutdoorTemp:     params.OutdoorAirTemp,
		Mode:            RawModeToString(params.Mode & 0xf),
		Stage:           params.Mode >> 5,
		FanMode:         RawFanModeToString(cfg.GetZonalField(zone, "FanMode").(uint8)),
		Hold:            hold,
		HeatSetpoint:    cfg.GetZonalField(zone, "HeatSetpoint").(uint8),
		CoolSetpoint:    cfg.GetZonalField(zone, "CoolSetpoint").(uint8),
		RawMode:         params.Mode,
	}, true
}

func (a *Api) GetTstatSettings() (*TStatSettings, bool) {
	tss := TStatSettings{}
	if !a.Protocol.ReadTable(DevTSTAT, &tss) {
		return nil, false
	}
	return &tss, true
}

func (a *Api) GetAirHandler() (AirHandler, bool) {
	b := a.Cache.Get(blowerCacheKey)
	tb, ok := b.(*AirHandler)
	if !ok {
		return AirHandler{}, false
	}
	return *tb, true
}

func (a *Api) GetHeatPump() (HeatPump, bool) {
	h := a.Cache.Get(heatpumpCacheKey)
	th, ok := h.(*HeatPump)
	if !ok {
		return HeatPump{}, false
	}
	return *th, true
}

func (a *Api) GetTableRaw(deviceAddr uint16, table []byte) []byte {
	var addr TableAddr
	copy(addr[:], table[0:3])
	raw := rawRequest{Data: &[]byte{}}

	if a.Protocol.Read(uint16(deviceAddr), addr, raw) {
		return *raw.Data
	}
	return nil
}

func (a *Api) UpdateThermostat(table Table, flags uint8) bool {
	return a.Protocol.WriteTable(DevTSTAT, table, flags)
}

func (a *Api) NewListener() *dispatcher.Listener {
	return a.dispatcher.NewListener()
}
