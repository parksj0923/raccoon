package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"raccoon/exchange"
	"raccoon/model"
	"raccoon/utils/log"
)

// ======= 타입 정의 =======

// TrendType은 시장 추세를 나타내는 열거형입니다.
type TrendType int

const (
	Bullish TrendType = iota
	Bearish
	Sideways
)

// Signal은 매매 신호를 나타냅니다.
type Signal struct {
	Date   string
	Action string
	Price  float64
	Trend  TrendType
}

// Trade는 개별 거래 정보를 나타냅니다.
type Trade struct {
	BuyDate    time.Time
	SellDate   time.Time
	BuyPrice   float64
	SellPrice  float64
	Return     float64
	Duration   time.Duration
	TradeType  string // "PARTIAL_BUY", "PARTIAL_SELL", "FINAL_SELL", "BUY", "SELL"
	Position   float64
	Balance    float64
	MaxBalance float64
}

// MonthlyTrend은 월별 추세 정보를 담습니다.
type MonthlyTrend struct {
	Month   time.Time
	Trend   TrendType
	Candles []model.Candle
}

// ======= 1. 시장 추세 분석 =======

// detectMonthlyTrends는 월별 추세를 분석합니다.
func detectMonthlyTrends(months []MonthlyTrend, threshold float64) {
	for idx := range months {
		if len(months[idx].Candles) == 0 {
			months[idx].Trend = Sideways
			continue
		}
		startPrice := months[idx].Candles[0].Close
		endPrice := months[idx].Candles[len(months[idx].Candles)-1].Close
		returnRate := (endPrice - startPrice) / startPrice * 100

		if returnRate >= threshold {
			months[idx].Trend = Bullish
		} else if returnRate <= -threshold {
			months[idx].Trend = Bearish
		} else {
			months[idx].Trend = Sideways
		}
	}
}

// ======= 2. 공통 지표 계산 함수 =======

// calculateMovingAverages는 단기 및 장기 이동평균을 계산합니다.
func calculateMovingAverages(data []model.Candle, shortWindow, longWindow int) ([]float64, []float64) {
	shortMA := make([]float64, len(data))
	longMA := make([]float64, len(data))

	for i := range data {
		if i >= shortWindow-1 {
			sum := 0.0
			for j := i; j > i-shortWindow; j-- {
				sum += data[j].Close
			}
			shortMA[i] = sum / float64(shortWindow)
		} else {
			shortMA[i] = math.NaN()
		}

		if i >= longWindow-1 {
			sum := 0.0
			for j := i; j > i-longWindow; j-- {
				sum += data[j].Close
			}
			longMA[i] = sum / float64(longWindow)
		} else {
			longMA[i] = math.NaN()
		}
	}

	return shortMA, longMA
}

// calculateRSI는 RSI(Relative Strength Index)를 계산합니다.
func calculateRSI(data []model.Candle, period int) []float64 {
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

// calculateEMA는 지수 이동평균(EMA)을 계산합니다.
func calculateEMA(data []model.Candle, window int) []float64 {
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

// calculateEMAFromSlice는 주어진 슬라이스에 대해 EMA를 계산합니다.
func calculateEMAFromSlice(data []float64, window int) []float64 {
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

// calculateMACD는 MACD 지표를 계산합니다.
func calculateMACD(data []model.Candle, shortWindow, longWindow, signalWindow int) ([]float64, []float64, []float64) {
	macd := make([]float64, len(data))
	signal := make([]float64, len(data))
	histogram := make([]float64, len(data))

	shortEMA := calculateEMA(data, shortWindow)
	longEMA := calculateEMA(data, longWindow)

	for i := 0; i < len(data); i++ {
		if !math.IsNaN(shortEMA[i]) && !math.IsNaN(longEMA[i]) {
			macd[i] = shortEMA[i] - longEMA[i]
		} else {
			macd[i] = math.NaN()
		}
	}

	// Signal Line 계산 (EMA of MACD)
	signal = calculateEMAFromSlice(macd, signalWindow)

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

// calculateStochasticOscillator는 스토캐스틱 오실레이터를 계산합니다.
func calculateStochasticOscillator(data []model.Candle, period int) ([]float64, []float64) {
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

// calculateBollingerBands는 볼린저 밴드를 계산합니다.
func calculateBollingerBands(data []model.Candle, period int, multiplier float64) (middleBand, upperBand, lowerBand []float64) {
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

// ======= 3. 매매 신호 생성 =======

// generateSignals는 시장 추세에 따라 매매 신호를 생성합니다.
func generateSignals(data []model.Candle, indicators map[string][]float64, trends []TrendType) []Signal {
	signals := []Signal{}
	signalMap := make(map[string]Signal) // 날짜별로 하나의 신호만 저장

	shortMA := indicators["shortMA"]
	longMA := indicators["longMA"]
	rsi := indicators["rsi"]
	middleBand := indicators["middleBand"]
	upperBand := indicators["upperBand"]
	lowerBand := indicators["lowerBand"]
	macd := indicators["macd"]
	signalMACD := indicators["signalMACD"]
	stochK := indicators["stochK"]
	stochD := indicators["stochD"]

	for i := 1; i < len(data); i++ {
		currentTrend := trends[i]
		dateStr := data[i].Time.Format(time.RFC3339)

		switch currentTrend {
		case Bullish:
			// 상승장 전략
			// 매수 조건:
			// 1. 단기 MA가 장기 MA를 상향 돌파
			// 2. RSI가 과매수 상태가 아님 (예: 30 < RSI < 70)
			// 3. 양봉 및 거래량 증가
			shouldBuy := false
			if !math.IsNaN(shortMA[i]) && !math.IsNaN(longMA[i]) && !math.IsNaN(rsi[i]) &&
				!math.IsNaN(middleBand[i]) && !math.IsNaN(lowerBand[i]) &&
				!math.IsNaN(macd[i]) && !math.IsNaN(signalMACD[i]) &&
				!math.IsNaN(stochK[i]) && !math.IsNaN(stochD[i]) {

				if (shortMA[i] > longMA[i] && shortMA[i-1] <= longMA[i-1]) &&
					(rsi[i] < 70) &&
					(data[i].Close > data[i].Open && data[i].Volume > data[i-1].Volume) {
					shouldBuy = true
				}

				// 추가 매수 조건: 최근 N개의 캔들 중 저점이 상승하는 추세에 있고, 현재 캔들이 양봉
				if i >= 3 {
					isLowIncreasing := data[i-2].Low < data[i-1].Low && data[i-1].Low < data[i].Low
					if isLowIncreasing && data[i].Close > data[i].Open && rsi[i] < 70 {
						shouldBuy = true
					}
				}
			}

			// 매도 조건:
			// 1. 단기 MA가 장기 MA를 하향 돌파
			// 2. RSI가 과매수 상태 (예: RSI > 70)
			shouldSell := false
			if !math.IsNaN(shortMA[i]) && !math.IsNaN(longMA[i]) && !math.IsNaN(rsi[i]) {
				if (shortMA[i] < longMA[i] && shortMA[i-1] >= longMA[i-1]) ||
					rsi[i] > 70 {
					shouldSell = true
				}

				// 추가 매도 조건: 최근 N개의 캔들 중 고점이 하락하는 추세에 있고, 현재 캔들이 음봉
				if i >= 3 {
					isHighDecreasing := data[i-2].High > data[i-1].High && data[i-1].High > data[i].High
					if isHighDecreasing && data[i].Close < data[i].Open && rsi[i] > 30 {
						shouldSell = true
					}
				}
			}

			if shouldSell {
				// 매도 신호가 이미 설정되어 있지 않다면 설정
				if existingSignal, exists := signalMap[dateStr]; !exists || existingSignal.Action != "SELL" {
					signalMap[dateStr] = Signal{
						Date:   dateStr,
						Action: "SELL",
						Price:  data[i].Close,
						Trend:  currentTrend,
					}
				}
			}

			if shouldBuy {
				// 매수 신호가 이미 설정되어 있지 않다면 설정
				if _, exists := signalMap[dateStr]; !exists {
					signalMap[dateStr] = Signal{
						Date:   dateStr,
						Action: "BUY",
						Price:  data[i].Close,
						Trend:  currentTrend,
					}
				}
			}

		case Sideways:
			// 횡보장 전략
			// 여기에는 횡보장에 맞는 신호 생성 로직을 구현합니다.
			// 예시로, 단순히 RSI와 볼린저 밴드를 사용하는 매매 조건을 적용합니다.

			shouldBuy := false
			shouldSell := false

			if !math.IsNaN(shortMA[i]) && !math.IsNaN(longMA[i]) && !math.IsNaN(rsi[i]) &&
				!math.IsNaN(middleBand[i]) && !math.IsNaN(upperBand[i]) && !math.IsNaN(lowerBand[i]) &&
				!math.IsNaN(macd[i]) && !math.IsNaN(signalMACD[i]) &&
				!math.IsNaN(stochK[i]) && !math.IsNaN(stochD[i]) {

				// 매수 조건:
				// 1. RSI가 과매도 상태 (예: RSI < 30)
				// 2. 가격이 하단 볼린저 밴드에 근접
				if rsi[i] < 30 && data[i].Close <= lowerBand[i]+(upperBand[i]-lowerBand[i])/2 {
					shouldBuy = true
				}

				// 매도 조건:
				// 1. RSI가 과매수 상태 (예: RSI > 70)
				// 2. 가격이 상단 볼린저 밴드에 근접
				if rsi[i] > 70 && data[i].Close >= upperBand[i]-(upperBand[i]-lowerBand[i])/2 {
					shouldSell = true
				}
			}

			if shouldSell {
				if existingSignal, exists := signalMap[dateStr]; !exists || existingSignal.Action != "SELL" {
					signalMap[dateStr] = Signal{
						Date:   dateStr,
						Action: "SELL",
						Price:  data[i].Close,
						Trend:  currentTrend,
					}
				}
			}

			if shouldBuy {
				if _, exists := signalMap[dateStr]; !exists {
					signalMap[dateStr] = Signal{
						Date:   dateStr,
						Action: "BUY",
						Price:  data[i].Close,
						Trend:  currentTrend,
					}
				}
			}

		case Bearish:
			// 하락장 전략
			// 매도 조건:
			// 1. 단기 MA가 장기 MA를 하향 돌파
			// 2. RSI가 과매수 상태 (예: RSI > 70)
			// 3. 가격이 상단 볼린저 밴드에 도달
			// 4. MACD가 시그널 라인을 하향 돌파
			// 5. 스토캐스틱 %K가 %D를 하향 돌파
			shouldSell := false
			if !math.IsNaN(shortMA[i]) && !math.IsNaN(longMA[i]) && !math.IsNaN(rsi[i]) &&
				!math.IsNaN(middleBand[i]) && !math.IsNaN(upperBand[i]) && !math.IsNaN(lowerBand[i]) &&
				!math.IsNaN(macd[i]) && !math.IsNaN(signalMACD[i]) &&
				!math.IsNaN(stochK[i]) && !math.IsNaN(stochD[i]) {

				if (shortMA[i] < longMA[i] && shortMA[i-1] >= longMA[i-1]) ||
					(rsi[i] > 70) ||
					(data[i].Close >= upperBand[i]) ||
					(macd[i] < signalMACD[i]) ||
					(stochK[i] < stochD[i] && stochK[i-1] >= stochD[i-1]) {
					shouldSell = true
				}

				// 추가 매도 조건: 최근 N개의 캔들 중 고점이 하락하는 추세에 있고, 현재 캔들이 음봉
				if i >= 3 {
					isHighDecreasing := data[i-2].High > data[i-1].High && data[i-1].High > data[i].High
					if isHighDecreasing && data[i].Close < data[i].Open && rsi[i] > 30 {
						shouldSell = true
					}
				}
			}

			// 매수 조건:
			// 1. 단기 MA가 장기 MA를 상향 돌파
			// 2. RSI가 과매수 상태가 아님
			// 3. 가격이 하단 볼린저 밴드에 도달
			// 4. MACD가 시그널 라인을 상향 돌파
			// 5. 스토캐스틱 %K가 %D를 상향 돌파
			shouldBuy := false
			if !math.IsNaN(shortMA[i]) && !math.IsNaN(longMA[i]) && !math.IsNaN(rsi[i]) &&
				!math.IsNaN(middleBand[i]) && !math.IsNaN(upperBand[i]) && !math.IsNaN(lowerBand[i]) &&
				!math.IsNaN(macd[i]) && !math.IsNaN(signalMACD[i]) &&
				!math.IsNaN(stochK[i]) && !math.IsNaN(stochD[i]) {

				if (shortMA[i] > longMA[i] && shortMA[i-1] <= longMA[i-1]) &&
					(rsi[i] < 70) &&
					(data[i].Close <= lowerBand[i]) &&
					(macd[i] > signalMACD[i] && macd[i-1] <= signalMACD[i-1]) &&
					(stochK[i] > stochD[i] && stochK[i-1] <= stochD[i-1]) {
					shouldBuy = true
				}

				// 추가 매수 조건: 최근 N개의 캔들 중 저점이 상승하는 추세에 있고, 현재 캔들이 양봉
				if i >= 3 {
					isLowIncreasing := data[i-2].Low < data[i-1].Low && data[i-1].Low < data[i].Low
					if isLowIncreasing && data[i].Close > data[i].Open && rsi[i] < 70 {
						shouldBuy = true
					}
				}
			}

			if shouldSell {
				// 매도 신호가 이미 설정되어 있지 않다면 설정
				if existingSignal, exists := signalMap[dateStr]; !exists || existingSignal.Action != "SELL" {
					signalMap[dateStr] = Signal{
						Date:   dateStr,
						Action: "SELL",
						Price:  data[i].Close,
						Trend:  currentTrend,
					}
				}
			}

			if shouldBuy {
				// 매수 신호가 이미 설정되어 있지 않다면 설정
				if _, exists := signalMap[dateStr]; !exists {
					signalMap[dateStr] = Signal{
						Date:   dateStr,
						Action: "BUY",
						Price:  data[i].Close,
						Trend:  currentTrend,
					}
				}
			}
		}
	}

	// 신호 맵을 슬라이스로 변환 (날짜 순으로 정렬)
	dates := make([]string, 0, len(signalMap))
	for date := range signalMap {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	for _, dateStr := range dates {
		if signal, exists := signalMap[dateStr]; exists {
			signals = append(signals, signal)
		}
	}

	return signals
}

// ======= 4. 백테스트 함수 =======

// backtest는 매매 신호를 기반으로 백테스트를 수행합니다.
func backtest(data []model.Candle, signals []Signal, initialBalance float64) float64 {
	balance := initialBalance
	position := 0.0
	buyPrice := 0.0
	buyDate := time.Time{}
	initialCapital := initialBalance
	maxBalance := initialBalance

	var previousReturn float64
	var firstTrade bool = true

	// 캔들 데이터를 날짜별로 빠르게 접근할 수 있도록 맵으로 변환
	candleMap := make(map[string]model.Candle)
	for _, candle := range data {
		dateStr := candle.Time.Format(time.RFC3339)
		candleMap[dateStr] = candle
	}

	trades := []Trade{}

	for _, signal := range signals {
		candle, exists := candleMap[signal.Date]
		if !exists {
			continue
		}

		if signal.Action == "BUY" {
			if balance >= candle.Close {
				position = balance / candle.Close
				buyPrice = candle.Close // 매수가격 저장
				buyDate = candle.Time
				balance = 0
				trades = append(trades, Trade{
					BuyDate:   buyDate,
					SellDate:  time.Time{},
					BuyPrice:  buyPrice,
					SellPrice: 0.0,
					Return:    0.0,
					Duration:  0,
					TradeType: "BUY",
					Position:  position,
					Balance:   balance,
				})
				fmt.Printf("매수: %s | 매수가: %.2f\n", candle.Time.Format("2006-01-02"), candle.Close)
			} else {
				fmt.Printf("매수 신호 발생: %s | 하지만 잔고가 충분하지 않습니다.\n", candle.Time.Format("2006-01-02"))
			}
		} else if signal.Action == "SELL" && position > 0 {
			// 매도 조건: 매도가격이 매수가격보다 높을 때 매도
			if candle.Close > buyPrice {
				balance = position * candle.Close
				position = 0
				trades = append(trades, Trade{
					BuyDate:   buyDate,
					SellDate:  candle.Time,
					BuyPrice:  buyPrice,
					SellPrice: candle.Close,
					Return:    ((candle.Close - buyPrice) / buyPrice) * 100,
					Duration:  candle.Time.Sub(buyDate),
					TradeType: "SELL",
					Position:  position,
					Balance:   balance,
				})
				fmt.Printf("매도: %s | 매도가: %.2f\n", candle.Time.Format("2006-01-02"), candle.Close)

				// 현재 수익률 계산
				currentReturn := ((candle.Close - buyPrice) / buyPrice) * 100
				fmt.Printf("현재 수익률: %.2f%%\n", currentReturn)

				// 이전 수익률과의 차이 계산
				if !firstTrade {
					deltaReturn := currentReturn - previousReturn
					fmt.Printf("직전 수익률 대비 차이: %.2f%%\n", deltaReturn)
				} else {
					firstTrade = false
					fmt.Println("직전 수익률 없음 (첫 번째 매도)")
				}

				// 이전 수익률 업데이트
				previousReturn = currentReturn

				if balance > maxBalance {
					maxBalance = balance
				}
			} else {
				fmt.Printf("매도 조건 발생: %s | 하지만 매도가 %.2f가 매수가 %.2f보다 낮아 판매하지 않음\n",
					candle.Time.Format("2006-01-02"), candle.Close, buyPrice)
			}
		}
	}

	// 최종 포지션 정리
	if position > 0 {
		finalCandle := data[len(data)-1]
		finalClose := finalCandle.Close
		finalDate := finalCandle.Time

		if finalClose > buyPrice {
			balance = position * finalClose
			position = 0
			trades = append(trades, Trade{
				BuyDate:   buyDate,
				SellDate:  finalDate,
				BuyPrice:  buyPrice,
				SellPrice: finalClose,
				Return:    ((finalClose - buyPrice) / buyPrice) * 100,
				Duration:  finalDate.Sub(buyDate),
				TradeType: "FINAL_SELL",
				Position:  position,
				Balance:   balance,
			})
			fmt.Printf("최종 매도: %s | 매도가: %.2f\n", finalDate.Format("2006-01-02"), finalClose)
			fmt.Printf("최종 수익률: %.2f%%\n", ((finalClose-buyPrice)/buyPrice)*100)
		} else {
			fmt.Printf("최종 포지션 보유 중: %s | 현재 가격: %.2f, 매수가: %.2f | 포지션 가치 반영: %.2f\n",
				finalDate.Format("2006-01-02"), finalClose, buyPrice, position*finalClose)
		}
	}

	// 거래 내역 출력
	fmt.Println("\n--- 거래 내역 ---")
	for idx, trade := range trades {
		if trade.TradeType == "BUY" {
			fmt.Printf("거래 %d: 매수\n", idx+1)
			fmt.Printf("  매수일: %s | 매수가: %.2f\n", trade.BuyDate.Format("2006-01-02"), trade.BuyPrice)
		} else if trade.TradeType == "SELL" || trade.TradeType == "FINAL_SELL" {
			fmt.Printf("거래 %d: 매도\n", idx+1)
			fmt.Printf("  매도일: %s | 매도가: %.2f\n", trade.SellDate.Format("2006-01-02"), trade.SellPrice)
			fmt.Printf("  수익률: %.2f%%\n", trade.Return)
			fmt.Printf("  거래 기간: %.0f 일\n", trade.Duration.Hours()/24)
		}
	}
	fmt.Println("-----------------")

	totalReturn := ((balance - initialCapital) / initialCapital) * 100
	fmt.Printf("\n최종 잔고: %.2f\n", balance)
	fmt.Printf("총 수익률: %.2f%%\n", totalReturn)
	fmt.Printf("최대 잔고: %.2f\n", maxBalance)
	return balance
}

// ======= 5. 메인 함수 =======

func main() {
	// 환경 변수에서 API 키 가져오기
	apiKey := os.Getenv("UPBIT_ACCESS_KEY")
	secretKey := os.Getenv("UPBIT_SECRET_KEY")
	pairs := []string{"KRW-DOGE"}
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		log.Fatal(err)
	}

	KSTloc, _ := time.LoadLocation("Asia/Seoul")
	start := time.Date(2021, time.January, 01, 00, 00, 0, 0, KSTloc)
	end := time.Date(2025, time.January, 23, 00, 00, 0, 0, KSTloc)
	data, err := upbit.CandlesByPeriod(pairs[0], "1d", start, end)
	if err != nil {
		log.Fatal(err)
	}

	// 월별로 데이터를 분류
	monthlyMap := make(map[string][]model.Candle)
	for _, candle := range data {
		monthStr := candle.Time.Format("2006-01")
		monthlyMap[monthStr] = append(monthlyMap[monthStr], candle)
	}

	// MonthlyTrend 슬라이스 생성
	months := []MonthlyTrend{}
	for monthStr, candles := range monthlyMap {
		monthTime, _ := time.Parse("2006-01", monthStr)
		months = append(months, MonthlyTrend{
			Month:   monthTime,
			Candles: candles,
		})
	}

	// 추세 감지
	threshold := 10.0 // 임계값 설정 (예: 10%)
	detectMonthlyTrends(months, threshold)

	// 전체 데이터에 대한 추세 할당 (매일의 추세를 월별로 할당)
	trends := make([]TrendType, len(data))
	for i, candle := range data {
		monthStr := candle.Time.Format("2006-01")
		for _, month := range months {
			if month.Month.Format("2006-01") == monthStr {
				trends[i] = month.Trend
				break
			}
		}
	}

	// 지표 계산을 위한 매개변수 설정
	shortWindow := 10          // 단기 이동평균 기간
	longWindow := 30           // 장기 이동평균 기간
	rsiPeriod := 14            // RSI 계산 기간
	bollingerPeriod := 20      // 볼린저 밴드 계산 기간
	bollingerMultiplier := 2.0 // 표준편차 배수
	macdShortWindow := 12      // MACD 단기 윈도우
	macdLongWindow := 26       // MACD 장기 윈도우
	macdSignalWindow := 9      // MACD 시그널 윈도우
	stochPeriod := 14          // 스토캐스틱 오실레이터 기간

	// 공통 지표 계산
	shortMA, longMA := calculateMovingAverages(data, shortWindow, longWindow)
	rsi := calculateRSI(data, rsiPeriod)
	middleBand, upperBand, lowerBand := calculateBollingerBands(data, bollingerPeriod, bollingerMultiplier)
	macd, signalMACD, histogram := calculateMACD(data, macdShortWindow, macdLongWindow, macdSignalWindow)
	stochK, stochD := calculateStochasticOscillator(data, stochPeriod)

	// 지표를 맵에 저장
	indicators := map[string][]float64{
		"shortMA":    shortMA,
		"longMA":     longMA,
		"rsi":        rsi,
		"middleBand": middleBand,
		"upperBand":  upperBand,
		"lowerBand":  lowerBand,
		"macd":       macd,
		"signalMACD": signalMACD,
		"histogram":  histogram,
		"stochK":     stochK,
		"stochD":     stochD,
	}

	// 매매 신호 생성
	signals := generateSignals(data, indicators, trends)
	fmt.Printf("총 매매 신호 수: %d\n", len(signals)) // 신호 수 출력

	// 백테스트 실행
	initialBalance := 1000000000.0 // 초기 자본 (예: 1,000,000,000 KRW)
	backtest(data, signals, initialBalance)
}
