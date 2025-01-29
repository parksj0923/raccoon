package indicator

import (
	"math"
	"raccoon/model"
	"sort"
	"time"
)

type TrendType int

const (
	Bullish TrendType = iota
	Bearish
	Sideways
)

type MonthlyTrend struct {
	Month   time.Time
	Trend   TrendType
	Candles []model.Candle
}

type MetricStyle string

const (
	StyleBar       = "bar"
	StyleScatter   = "scatter"
	StyleLine      = "line"
	StyleHistogram = "histogram"
	StyleWaterfall = "waterfall"
)

type IndicatorMetric struct {
	Name   string
	Color  string
	Style  MetricStyle // default: line
	Values model.Series[float64]
}

type ChartIndicator struct {
	Time      []time.Time
	Metrics   []IndicatorMetric
	Overlay   bool
	GroupName string
	Warmup    int
}

func CustomSMA(data []model.Candle, window int) []float64 {
	simpleMA := make([]float64, len(data))
	for i := range data {
		if i >= window-1 {
			sum := 0.0
			for j := i; j > i-window; j-- {
				sum += data[j].Close
			}
			simpleMA[i] = sum / float64(window)
		} else {
			simpleMA[i] = math.NaN()
		}
	}
	return simpleMA
}

func CustomRSI(data []model.Candle, period int) []float64 {
	rsi := make([]float64, len(data))
	gains := make([]float64, len(data))
	losses := make([]float64, len(data))

	for i := 1; i < len(data); i++ {
		change := data[i].Close - data[i-1].Close
		if change > 0 {
			gains[i] = change
		} else {
			losses[i] = -change
		}
	}

	for i := period; i < len(data); i++ {
		avgGain := 0.0
		avgLoss := 0.0
		for j := i - period + 1; j <= i; j++ {
			avgGain += gains[j]
			avgLoss += losses[j]
		}
		avgGain /= float64(period)
		avgLoss /= float64(period)

		if avgLoss == 0 {
			rsi[i] = 100
		} else {
			rs := avgGain / avgLoss
			rsi[i] = 100 - (100 / (1 + rs))
		}
	}

	return rsi
}

func CustomEMA(data []model.Candle, window int) []float64 {
	ema := make([]float64, len(data))
	k := 2.0 / (float64(window) + 1)

	var previousEMA float64
	for i := 0; i < len(data); i++ {
		if i == window-1 {
			sum := 0.0
			for j := i - window + 1; j <= i; j++ {
				sum += data[j].Close
			}
			previousEMA = sum / float64(window)
			ema[i] = previousEMA
		} else if i >= window {
			if !math.IsNaN(data[i].Close) && !math.IsNaN(previousEMA) {
				previousEMA = (data[i].Close-previousEMA)*k + previousEMA
				ema[i] = previousEMA
			} else {
				ema[i] = math.NaN()
			}
		} else {
			ema[i] = math.NaN()
		}
	}
	return ema
}

func CustomEMAFromSlice(data []float64, window int) []float64 {
	ema := make([]float64, len(data))
	k := 2.0 / (float64(window) + 1)

	var previousEMA float64
	for i := 0; i < len(data); i++ {
		if i == window-1 {
			sum := 0.0
			validCount := 0
			for j := i - window + 1; j <= i; j++ {
				if !math.IsNaN(data[j]) {
					sum += data[j]
					validCount++
				}
			}
			if validCount > 0 {
				previousEMA = sum / float64(validCount)
				ema[i] = previousEMA
			} else {
				ema[i] = math.NaN()
			}
		} else if i >= window {
			if !math.IsNaN(data[i]) && !math.IsNaN(previousEMA) {
				previousEMA = (data[i]-previousEMA)*k + previousEMA
				ema[i] = previousEMA
			} else {
				ema[i] = math.NaN()
			}
		} else {
			ema[i] = math.NaN()
		}
	}
	return ema
}

func CustomMACD(data []model.Candle, shortWindow, longWindow, signalWindow int) ([]float64, []float64, []float64) {
	macd := make([]float64, len(data))
	signal := make([]float64, len(data))
	histogram := make([]float64, len(data))

	shortEMA := CustomEMA(data, shortWindow)
	longEMA := CustomEMA(data, longWindow)

	for i := 0; i < len(data); i++ {
		if !math.IsNaN(shortEMA[i]) && !math.IsNaN(longEMA[i]) {
			macd[i] = shortEMA[i] - longEMA[i]
		} else {
			macd[i] = math.NaN()
		}
	}

	// Signal Line 계산 (EMA of MACD)
	signal = CustomEMAFromSlice(macd, signalWindow)

	// Histogram 계산
	for i := 0; i < len(data); i++ {
		if !math.IsNaN(macd[i]) && !math.IsNaN(signal[i]) {
			histogram[i] = macd[i] - signal[i]
		} else {
			histogram[i] = math.NaN()
		}
	}

	return macd, signal, histogram
}

func CustomStochasticOscillator(data []model.Candle, period int) ([]float64, []float64) {
	stochK := make([]float64, len(data))
	stochD := make([]float64, len(data))

	for i := period - 1; i < len(data); i++ {
		lowestLow := data[i].Low
		highestHigh := data[i].High
		for j := i - period + 1; j <= i; j++ {
			if data[j].Low < lowestLow {
				lowestLow = data[j].Low
			}
			if data[j].High > highestHigh {
				highestHigh = data[j].High
			}
		}
		if highestHigh != lowestLow {
			stochK[i] = (data[i].Close - lowestLow) / (highestHigh - lowestLow) * 100
		} else {
			stochK[i] = 50
		}
	}

	// %D 라인 계산 (3일 이동평균)
	for i := 2; i < len(data); i++ {
		if !math.IsNaN(stochK[i]) && !math.IsNaN(stochK[i-1]) && !math.IsNaN(stochK[i-2]) {
			stochD[i] = (stochK[i] + stochK[i-1] + stochK[i-2]) / 3
		} else {
			stochD[i] = math.NaN()
		}
	}

	return stochK, stochD
}

func CustomBollingerBands(data []model.Candle, period int, multiplier float64) (middleBand, upperBand, lowerBand []float64) {
	n := len(data)
	middleBand = make([]float64, n)
	upperBand = make([]float64, n)
	lowerBand = make([]float64, n)

	for i := period - 1; i < n; i++ {
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			sum += data[j].Close
		}
		ma := sum / float64(period)
		middleBand[i] = ma

		// 표준편차 계산
		variance := 0.0
		for j := i - period + 1; j <= i; j++ {
			variance += math.Pow(data[j].Close-ma, 2)
		}
		stdDev := math.Sqrt(variance / float64(period))

		upperBand[i] = ma + (multiplier * stdDev)
		lowerBand[i] = ma - (multiplier * stdDev)
	}

	// 초기값은 NaN으로 설정
	for i := 0; i < period-1; i++ {
		middleBand[i] = math.NaN()
		upperBand[i] = math.NaN()
		lowerBand[i] = math.NaN()
	}

	return middleBand, upperBand, lowerBand
}

// DetectMonthlyTrendsDF : df 전체를 월별로 묶어, 각 월의 상승/하락/횡보 추세를 계산한 다음,
//
//	df의 각 봉(일자)이 어느 월에 속하는지 찾아서 TrendType 배열을 리턴
//
// threshold(%) 이상 상승이면 Bullish, 이하로 하락이면 Bearish, 나머지는 Sideways 로 분류
func DetectMonthlyTrendsDF(df *model.Dataframe, threshold float64) []TrendType {
	n := len(df.Close)
	if n == 0 {
		return nil
	}

	// 1) 월별로 캔들을 묶기 위해 map 구성
	monthlyMap := make(map[string][]model.Candle)
	for i := 0; i < n; i++ {
		yyyymm := df.Time[i].Format("2006-01")
		c := model.Candle{
			Time:   df.Time[i],
			Open:   df.Open[i],
			High:   df.High[i],
			Low:    df.Low[i],
			Close:  df.Close[i],
			Volume: df.Volume[i],
		}
		monthlyMap[yyyymm] = append(monthlyMap[yyyymm], c)
	}

	// 2) MonthlyTrend 슬라이스를 만들어 추세 계산
	var months []MonthlyTrend
	for yyyymm, candles := range monthlyMap {
		mt, _ := time.Parse("2006-01", yyyymm)
		months = append(months, MonthlyTrend{
			Month:   mt,
			Candles: candles,
		})
	}
	// 월이 오름차순(오래된 순)으로 정렬되도록
	sort.Slice(months, func(i, j int) bool {
		return months[i].Month.Before(months[j].Month)
	})

	// 3) 실제 추세 감지
	for i := range months {
		if len(months[i].Candles) == 0 {
			months[i].Trend = Sideways
			continue
		}
		startPrice := months[i].Candles[0].Close
		endPrice := months[i].Candles[len(months[i].Candles)-1].Close
		if math.Abs(startPrice) < 1e-8 {
			// 혹시 0에 가까우면 그냥 Sideways 처리(에러 방지)
			months[i].Trend = Sideways
			continue
		}
		returnRate := (endPrice - startPrice) / startPrice * 100.0

		switch {
		case returnRate >= threshold:
			months[i].Trend = Bullish
		case returnRate <= -threshold:
			months[i].Trend = Bearish
		default:
			months[i].Trend = Sideways
		}
	}

	// 4) df의 각 봉이 어떤 월에 속하는지 찾아서 TrendType을 할당
	trendArr := make([]TrendType, n)
	for i := 0; i < n; i++ {
		yyyymm := df.Time[i].Format("2006-01")
		trendArr[i] = Sideways // 기본값
		for _, m := range months {
			if m.Month.Format("2006-01") == yyyymm {
				trendArr[i] = m.Trend
				break
			}
		}
	}

	return trendArr
}
