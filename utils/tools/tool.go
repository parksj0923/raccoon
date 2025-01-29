package tools

import (
	"fmt"
	"time"
)

func MapPeriodToCandleEndpoint(period string) (string, error) {
	switch period {
	case "1s":
		return "seconds", nil
	case "1m":
		return "minutes/1", nil
	case "3m":
		return "minutes/3", nil
	case "5m":
		return "minutes/5", nil
	case "10m":
		return "minutes/10", nil
	case "15m":
		return "minutes/15", nil
	case "30m":
		return "minutes/30", nil
	case "60m", "1h":
		return "minutes/60", nil
	case "240m":
		return "minutes/240", nil
	case "1d":
		return "days", nil
	case "1w":
		return "weeks", nil
	case "1M":
		return "months", nil
	case "1y":
		return "years", nil
	default:
		return "", fmt.Errorf("unsupported upbit period: %s", period)
	}
}

func ParseTimeframeToDuration(tf string) (time.Duration, error) {
	switch tf {
	case "1s":
		return 0, nil
	case "1m":
		return time.Minute, nil
	case "3m":
		return 3 * time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "10":
		return 10 * time.Second, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "60m", "1h":
		return time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "1w":
		return 7 * 24 * time.Hour, nil
	case "1M":
		return 30 * 24 * time.Hour, nil
	case "1y":
		return 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported timeframe: %s", tf)
	}
}
