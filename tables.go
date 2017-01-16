package main

type InfinityTableAddr [3]byte
type InfinityTable interface {
	addr() InfinityTableAddr
}

type TStatCurrentParams struct {
	Z1CurrentTemp     uint8
	Z2CurrentTemp     uint8
	Z3CurrentTemp     uint8
	Z4CurrentTemp     uint8
	Z5CurrentTemp     uint8
	Z6CurrentTemp     uint8
	Z7CurrentTemp     uint8
	Z8CurrentTemp     uint8
	Z1CurrentHumidity uint8
	Z2CurrentHumidity uint8
	Z3CurrentHumidity uint8
	Z4CurrentHumidity uint8
	Z5CurrentHumidity uint8
	Z6CurrentHumidity uint8
	Z7CurrentHumidity uint8
	Z8CurrentHumidity uint8
	Unknown1          uint8
	OutdoorAirTemp    uint8
	ZoneUnocc         uint8 // bitflags
	Mode              uint8
	Unknown2          [5]uint8
	DisplayedZone     uint8
}

func (params TStatCurrentParams) addr() InfinityTableAddr {
	return InfinityTableAddr{0x00, 0x3B, 0x02}
}

type TStatZoneParams struct {
	Z1FanMode        uint8
	Z2FanMode        uint8
	Z3FanMode        uint8
	Z4FanMode        uint8
	Z5FanMode        uint8
	Z6FanMode        uint8
	Z7FanMode        uint8
	Z8FanMode        uint8
	ZoneHold         uint8 // bitflags
	Z1HeatSetpoint   uint8
	Z2HeatSetpoint   uint8
	Z3HeatSetpoint   uint8
	Z4HeatSetpoint   uint8
	Z5HeatSetpoint   uint8
	Z6HeatSetpoint   uint8
	Z7HeatSetpoint   uint8
	Z8HeatSetpoint   uint8
	Z1CoolSetpoint   uint8
	Z2CoolSetpoint   uint8
	Z3CoolSetpoint   uint8
	Z4CoolSetpoint   uint8
	Z5CoolSetpoint   uint8
	Z6CoolSetpoint   uint8
	Z7CoolSetpoint   uint8
	Z8CoolSetpoint   uint8
	Z1TargetHumidity uint8
	Z2TargetHumidity uint8
	Z3TargetHumidity uint8
	Z4TargetHumidity uint8
	Z5TargetHumidity uint8
	Z6TargetHumidity uint8
	Z7TargetHumidity uint8
	Z8TargetHumidity uint8
	FanAutoCfg       uint8
	Unknown          uint8
	Z1HoldDuration   uint16
	Z2HoldDuration   uint16
	Z3HoldDuration   uint16
	Z4HoldDuration   uint16
	Z5HoldDuration   uint16
	Z6HoldDuration   uint16
	Z7HoldDuration   uint16
	Z8HoldDuration   uint16
	Z1Name           [12]byte
	Z2Name           [12]byte
	Z3Name           [12]byte
	Z4Name           [12]byte
	Z5Name           [12]byte
	Z6Name           [12]byte
	Z7Name           [12]byte
	Z8Name           [12]byte
}

func (params TStatZoneParams) addr() InfinityTableAddr {
	return InfinityTableAddr{0x00, 0x3B, 0x03}
}

type TStatVacationParams struct {
	Active         uint8
	Hours          uint16
	MinTemperature uint8
	MaxTemperature uint8
	MinHumidity    uint8
	MaxHumidity    uint8
	FanMode        uint8 // matches fan mode from TStatZoneParams
}

func (params TStatVacationParams) addr() InfinityTableAddr {
	return InfinityTableAddr{0x00, 0x3B, 0x04}
}

type APIVacationConfig struct {
	Active         *bool   `json:"active"`
	Days           *uint8  `json:"days"`
	MinTemperature *uint8  `json:"minTemperature"`
	MaxTemperature *uint8  `json:"maxTemperature"`
	MinHumidity    *uint8  `json:"minHumidity"`
	MaxHumidity    *uint8  `json:"maxHumidity"`
	FanMode        *string `json:"fanMode"`
}

func (params TStatVacationParams) toAPI() APIVacationConfig {
	api := APIVacationConfig{MinTemperature: &params.MinTemperature,
		MaxTemperature: &params.MaxTemperature,
		MinHumidity:    &params.MinHumidity,
		MaxHumidity:    &params.MaxHumidity}

	active := bool(params.Active == 1)
	api.Active = &active

	days := uint8(params.Hours / 7)
	api.Days = &days

	mode := rawFanModeToString(params.FanMode)
	api.FanMode = &mode

	return api
}

func (params *TStatVacationParams) fromAPI(config *APIVacationConfig) byte {
	flags := byte(0)

	if config.Days != nil {
		params.Hours = uint16(*config.Days) * uint16(24)
		flags |= 0x02
	}

	if config.MinTemperature != nil {
		params.MinTemperature = *config.MinTemperature
		flags |= 0x04
	}

	if config.MaxTemperature != nil {
		params.MaxTemperature = *config.MaxTemperature
		flags |= 0x08
	}

	if config.MinHumidity != nil {
		params.MinHumidity = *config.MinHumidity
		flags |= 0x10
	}

	if config.MaxHumidity != nil {
		params.MaxHumidity = *config.MaxHumidity
		flags |= 0x20
	}

	if config.FanMode != nil {
		mode, _ := stringFanModeToRaw(*config.FanMode)
		// FIXME: check for ok here
		params.FanMode = mode
		flags |= 0x40
	}

	return flags
}
