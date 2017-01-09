package main

func rawModeToString(mode uint8) string {
	switch mode {
	case 0:
		return "heat"
	case 1:
		return "cool"
	case 2:
		return "auto"
	case 5:
		return "off"
	default:
		return "unknown"
	}
}

func stringModeToRaw(mode string) uint8 {
	switch mode {
	case "heat":
		return 0
	case "cool":
		return 1
	case "auto":
		return 2
	case "off":
		return 5
	default:
		return 5
	}
}

func rawFanModeToString(mode uint8) string {
	switch mode {
	case 0:
		return "auto"
	case 1:
		return "low"
	case 2:
		return "med"
	case 3:
		return "high"
	default:
		return "unknown"
	}
}

func stringFanModeToRaw(mode string) (uint8, bool) {
	switch mode {
	case "auto":
		return 0, true
	case "low":
		return 1, true
	case "med":
		return 2, true
	case "high":
		return 3, true
	default:
		return 0, false
	}
}
