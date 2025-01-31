package strategy

import (
	"math"
	"raccoon/feed"
	"raccoon/indicator"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

type PSHStrategy struct {
	orderFeed *feed.OrderFeedSubscription
}

func NewPSHStrategy(orderFeed *feed.OrderFeedSubscription) *PSHStrategy {
	return &PSHStrategy{
		orderFeed: orderFeed,
	}
}

// Timeframe : 일봉 기준
func (s *PSHStrategy) Timeframe() string {
	return "1h"
}

// WarmupPeriod : 월별 추세 분석 등 지표 계산에 필요한 최소 봉 수
func (s *PSHStrategy) WarmupPeriod() int {
	return 60
}

// Indicators : 각 봉 완료 시(또는 백테스트 루프 내) 지표 계산.
// 여기서는 DetectMonthlyTrendsDF를 호출해 월별 추세를 구한 뒤, df.Metadata에 저장하고,
// 추가로 EMA, RSI, BollingerBands 등도 예시로 계산해서 저장합니다.
func (s *PSHStrategy) Indicators(df *model.Dataframe) []indicator.ChartIndicator {
	n := len(df.Close)
	if n == 0 {
		return nil
	}

	// --- (1) 월별 추세 계산 ---
	trendArr := indicator.DetectMonthlyTrendsDF(df, 10.0) // threshold=10%
	floatTrend := make([]float64, n)
	for i := 0; i < n; i++ {
		floatTrend[i] = float64(trendArr[i]) // Bullish=0, Bearish=1, Sideways=2 (enum-like)
	}
	df.Metadata["trend"] = floatTrend
	allCandles := dfToCandles(df)

	df.Metadata["shortMA"] = indicator.CustomEMA(allCandles, 10)
	df.Metadata["longMA"] = indicator.CustomEMA(allCandles, 30)
	df.Metadata["rsi"] = indicator.CustomRSI(allCandles, 14)
	df.Metadata["bb_mid"], df.Metadata["bb_up"], df.Metadata["bb_low"] =
		indicator.CustomBollingerBands(allCandles, 20, 2.0)

	df.Metadata["macd"], df.Metadata["macdSignal"], df.Metadata["macdHist"] =
		indicator.CustomMACD(allCandles, 12, 26, 9)

	df.Metadata["stochK"], df.Metadata["stochD"] =
		indicator.CustomStochasticOscillator(allCandles, 14)

	return []indicator.ChartIndicator{
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{
					Name:   "shortMA",
					Color:  "red",
					Style:  indicator.StyleLine,
					Values: df.Metadata["shortMA"],
				},
				{
					Name:   "longMA",
					Color:  "blue",
					Style:  indicator.StyleLine,
					Values: df.Metadata["longMA"],
				},
			},
			Overlay:   true,
			GroupName: "SMA",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{
					Name:   "MACD",
					Color:  "blue",
					Style:  indicator.StyleLine,
					Values: df.Metadata["macd"],
				},
				{
					Name:   "MACD Signal",
					Color:  "red",
					Style:  indicator.StyleLine,
					Values: df.Metadata["macdSignal"],
				},
				{
					Name:   "MACD Hist",
					Color:  "green",
					Style:  indicator.StyleHistogram,
					Values: df.Metadata["macdHist"],
				},
			},
			Overlay:   false, // MACD 보통 하단 분리차트 (false)
			GroupName: "MACD",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{
					Name:   "BB Upper",
					Color:  "gray",
					Style:  indicator.StyleLine,
					Values: df.Metadata["bb_up"],
				},
				{
					Name:   "BB Mid",
					Color:  "gray",
					Style:  indicator.StyleLine,
					Values: df.Metadata["bb_mid"],
				},
				{
					Name:   "BB Lower",
					Color:  "gray",
					Style:  indicator.StyleLine,
					Values: df.Metadata["bb_low"],
				},
			},
			Overlay:   true, // 볼린저밴드는 보통 종가 위 오버레이
			GroupName: "Bollinger",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{
					Name:   "RSI",
					Color:  "purple",
					Style:  indicator.StyleLine,
					Values: df.Metadata["rsi14"],
				},
			},
			Overlay:   true, // 볼린저밴드는 보통 종가 위 오버레이
			GroupName: "RSI",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{
					Name:   "stochK",
					Color:  "red",
					Style:  indicator.StyleLine,
					Values: df.Metadata["stochK"],
				},
				{
					Name:   "stochD",
					Color:  "blue",
					Style:  indicator.StyleLine,
					Values: df.Metadata["stochD"],
				},
			},
			Overlay:   true,
			GroupName: "Stochastic",
			Warmup:    s.WarmupPeriod(),
		},
	}
}

// OnCandle : 각 캔들 완료 시(또는 백테스트 루프에서) 불리는 함수. 여기서 추세와 지표 등을 활용해 매매 로직을 구현.
func (s *PSHStrategy) OnCandle(df *model.Dataframe, broker interfaces.Broker) {
	i := len(df.Close) - 1
	if i < s.WarmupPeriod()-1 {
		// 아직 지표를 계산하기에 봉 수가 부족하면 매매 스킵
		return
	}

	shortMA := df.Metadata["shortMA"]
	longMA := df.Metadata["longMA"]
	middleBand := df.Metadata["bb_mid"]
	upperBand := df.Metadata["bb_up"]
	lowerBand := df.Metadata["bb_low"]
	trendType := df.Metadata["trend"]
	macd := df.Metadata["macd"]
	macdSig := df.Metadata["macdSignal"]
	rsi := df.Metadata["rsi"]
	stochK := df.Metadata["stochK"]
	stochD := df.Metadata["stochD"]

	if trendType == nil || macd == nil || macdSig == nil || rsi == nil {
		log.Warn("[PSHStrategy] missing some indicators in Metadata")
		return
	}

	currentTrend := indicator.TrendType(int(trendType[i]))
	shortMAVal := shortMA[i]
	longMAVal := longMA[i]
	middleBandVal := middleBand[i]
	upperBandVal := upperBand[i]
	lowerBandVal := lowerBand[i]
	macdVal := macd[i]
	macdSigVal := macdSig[i]
	rsiVal := rsi[i]
	stochKVal := stochK[i]
	stochDVal := stochD[i]

	closePrice := df.Close[i]
	openPrice := df.Open[i]
	volume := df.Volume[i]

	// 포지션 조회 (잔고 확인용)
	coinAmt, krwAmt, _, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	// ----- (3) 단순 매매 시그널 예시 -----
	shouldBuy := false
	shouldSell := false

	switch currentTrend {
	case indicator.Bullish:
		// 상승장 전략

		if math.IsNaN(rsiVal) || math.IsNaN(shortMAVal) || math.IsNaN(longMAVal) {
			return
		}

		// 매수 조건:
		// 1. 단기 MA가 장기 MA를 상향 돌파
		// 2. RSI가 과매수 상태가 아님 (예: 30 < RSI < 70)
		// 3. 양봉 및 거래량 증가

		if ((shortMAVal > longMAVal) && (shortMA[i-1] <= longMA[i-1])) &&
			(rsiVal < 70) &&
			(closePrice > openPrice) && (volume > df.Volume[i-1]) {
			shouldBuy = true
		}

		// 추가 매수 조건: 최근 N개의 캔들 중 저점이 상승하는 추세에 있고, 현재 캔들이 양봉
		isLowIncreasing := (df.Low[i-2] < df.Low[i-1]) && (df.Low[i-1] < df.Low[i])
		if isLowIncreasing && (closePrice > openPrice) && rsiVal < 70 {
			shouldBuy = true
		}

		// 매도 조건:
		// 1. 단기 MA가 장기 MA를 하향 돌파
		// 2. RSI가 과매수 상태 (예: RSI > 70)

		if (shortMAVal < longMAVal) && (shortMA[i-1] >= longMA[i-1]) ||
			(rsiVal > 70) {
			shouldSell = true
		}

		// 추가 매도 조건: 최근 N개의 캔들 중 고점이 하락하는 추세에 있고, 현재 캔들이 음봉
		isHighDecreasing := (df.High[i-2] > df.High[i-1]) && (df.High[i-1] > df.High[i])
		if isHighDecreasing && (closePrice < openPrice) && (rsiVal > 30) {
			shouldSell = true
		}

	case indicator.Bearish:
		// 하락장 전략

		if math.IsNaN(rsiVal) || math.IsNaN(shortMAVal) || math.IsNaN(longMAVal) ||
			math.IsNaN(middleBandVal) || math.IsNaN(upperBandVal) || math.IsNaN(lowerBandVal) ||
			math.IsNaN(macdVal) || math.IsNaN(macdSigVal) ||
			math.IsNaN(stochKVal) || math.IsNaN(stochDVal) {
			return
		}

		// 매도 조건:
		// 1. 단기 MA가 장기 MA를 하향 돌파
		// 2. RSI가 과매수 상태 (예: RSI > 70)
		// 3. 가격이 상단 볼린저 밴드에 도달
		// 4. MACD가 시그널 라인을 하향 돌파
		// 5. 스토캐스틱 %K가 %D를 하향 돌파

		if ((shortMAVal < longMAVal) && (shortMA[i-1] >= longMA[i-1])) ||
			(rsi[i] > 70) ||
			(closePrice >= upperBandVal) ||
			(macdVal < macdSigVal) ||
			((stochKVal < stochDVal) && (stochK[i-1] >= stochD[i-1])) {
			shouldSell = true
		}

		// 추가 매도 조건: 최근 N개의 캔들 중 고점이 하락하는 추세에 있고, 현재 캔들이 음봉
		isHighDecreasing := (df.High[i-2] > df.High[i-1]) && (df.High[i-1] > df.High[i])
		if isHighDecreasing && (closePrice < openPrice) && (rsi[i] > 30) {
			shouldSell = true
		}

		// 매수 조건:
		// 1. 단기 MA가 장기 MA를 상향 돌파
		// 2. RSI가 과매수 상태가 아님
		// 3. 가격이 하단 볼린저 밴드에 도달
		// 4. MACD가 시그널 라인을 상향 돌파
		// 5. 스토캐스틱 %K가 %D를 상향 돌파

		if ((shortMAVal > longMAVal) && (shortMA[i-1] <= longMA[i-1])) &&
			(rsiVal < 70) &&
			(closePrice <= lowerBandVal) &&
			((macdVal > macdSigVal) && (macd[i-1] <= macdSig[i-1])) &&
			((stochKVal > stochDVal) && (stochK[i-1] <= stochD[i-1])) {
			shouldBuy = true
		}

		// 추가 매도 조건: 최근 N개의 캔들 중 고점이 하락하는 추세에 있고, 현재 캔들이 음봉
		isLowIncreasing := (df.Low[i-2] < df.Low[i-1]) && (df.Low[i-1] < df.Low[i])
		if isLowIncreasing && (closePrice > openPrice) && (rsiVal < 70) {
			shouldBuy = true
		}

	case indicator.Sideways:
		// 박스권 전략
		if math.IsNaN(rsiVal) ||
			math.IsNaN(middleBandVal) || math.IsNaN(upperBandVal) || math.IsNaN(lowerBandVal) {
			return
		}

		// 매수 조건:
		// 1. RSI가 과매도 상태 (예: RSI < 30)
		// 2. 가격이 하단 볼린저 밴드에 근접
		if (rsiVal < 30) && (closePrice <= (lowerBandVal + (upperBandVal-lowerBandVal)/2)) {
			shouldBuy = true
		}

		// 매도 조건:
		// 1. RSI가 과매수 상태 (예: RSI > 70)
		// 2. 가격이 상단 볼린저 밴드에 근접
		if (rsiVal > 70) && (closePrice >= (upperBandVal - (upperBandVal-lowerBandVal)/2)) {
			shouldSell = true
		}
	}

	if shouldSell {
		if coinAmt > 0 {
			// 예: "시장가 매도, 보유코인 전량"
			sellOrder := model.Order{
				Pair:       df.Pair,
				Side:       model.SideTypeSell,
				Type:       model.OrderTypeMarket,
				Quantity:   coinAmt,
				Price:      0,
				ExchangeID: "",
			}
			s.orderFeed.Publish(sellOrder)
			log.Infof("[PSHStrategy] 매도신호 -> OrderFeed.Publish(SELL %.8f)", coinAmt)
		}
	}

	if shouldBuy {
		//TODO 코인마다 최소 구매금액을 넣어야함
		if krwAmt >= 5000 {
			// 예: "시장가 매수, KRW 전액 사용" (Upbit가정: 시장가 매수 시 'price' param에 KRW금액)
			buyOrder := model.Order{
				Pair:     df.Pair,
				Side:     model.SideTypeBuy,
				Type:     model.OrderTypePrice, // upbit 시장가 매수
				Price:    krwAmt,               // KRW 전액
				Quantity: 0,                    // 수량은 빈값
				ID:       0,                    // 나중에 채워질 수 있음
			}
			s.orderFeed.Publish(buyOrder)
			log.Infof("[PSHStrategy] 매수신호 -> OrderFeed.Publish(BUY %.2fKRW)", krwAmt)
		} else {
			log.Infof("[PSHStrategy] 매수신호 -> 하지만 잔고가 충분하지 않음(잔고 %.2fKRW)", krwAmt)
		}
	}
}

// dfToCandles : DataFrame -> []model.Candle 변환 헬퍼
func dfToCandles(df *model.Dataframe) []model.Candle {
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
