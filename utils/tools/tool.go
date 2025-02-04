package tools

import (
	"fmt"
	"raccoon/model"
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
	case "240m", "4h":
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
	case "10m":
		return 10 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "60m", "1h":
		return time.Hour, nil
	case "240m", "4h":
		return 4 * time.Hour, nil
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

// 기존의 time.truncate 함수는 UTC기준으로 하기때문에 변경이 필요함
func TruncateKST(t time.Time, d time.Duration) (time.Time, error) {
	// KST 시간대 로드
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load KST location: %v", err)
	}

	// 시간을 KST 시간대로 변환
	local := t.In(loc)

	// 자정 기준 설정
	year, month, day := local.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, loc)

	// 자정부터 현재 시간까지의 경과 시간 계산
	elapsed := local.Sub(midnight)

	// 경과 시간을 주어진 duration으로 잘라냄
	truncatedElapsed := elapsed.Truncate(d)

	// 최종 truncated 시간 계산
	truncated := midnight.Add(truncatedElapsed)

	return truncated, nil
}

func DfToCandles(df *model.Dataframe) []model.Candle {
	out := make([]model.Candle, len(df.Close))
	for i := range df.Close {
		out[i] = model.Candle{
			Time:   df.Time[i],
			Open:   df.Open[i],
			High:   df.High[i],
			Low:    df.Low[i],
			Close:  df.Close[i],
			Volume: df.Volume[i],
		}
	}
	return out
}
