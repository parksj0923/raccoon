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
	return "1d"
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
	// df.Metadata는 map[string][]float64 형태로 관리된다고 가정
	floatTrend := make([]float64, n)
	for i := 0; i < n; i++ {
		floatTrend[i] = float64(trendArr[i]) // Bullish=0, Bearish=1, Sideways=2 (enum-like)
	}
	df.Metadata["trend"] = floatTrend
	allCandles := dfToCandles(df)

	df.Metadata["shortMA"] = indicator.CustomEMA(allCandles, 10)
	df.Metadata["longMA"] = indicator.CustomEMA(allCandles, 30)
	df.Metadata["rsi14"] = indicator.CustomRSI(allCandles, 14)
	df.Metadata["bb_mid"], df.Metadata["bb_up"], df.Metadata["bb_low"] =
		indicator.CustomBollingerBands(allCandles, 20, 2.0)

	df.Metadata["macd"], df.Metadata["macdSignal"], df.Metadata["macdHist"] =
		indicator.CustomMACD(allCandles, 12, 26, 9)

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
			GroupName: "PSHIndicators",
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
	}
}

// OnCandle : 각 캔들 완료 시(또는 백테스트 루프에서) 불리는 함수. 여기서 추세와 지표 등을 활용해 매매 로직을 구현.
func (s *PSHStrategy) OnCandle(df *model.Dataframe, broker interfaces.Broker) {
	i := len(df.Close) - 1
	if i < s.WarmupPeriod() {
		// 아직 지표를 계산하기에 봉 수가 부족하면 매매 스킵
		return
	}

	trendTypeArr := df.Metadata["trend"]
	macdArr := df.Metadata["macd"]
	macdSigArr := df.Metadata["macdSignal"]
	rsiArr := df.Metadata["rsi14"]

	if trendTypeArr == nil || macdArr == nil || macdSigArr == nil || rsiArr == nil {
		log.Warn("[PSHStrategy] missing some indicators in Metadata")
		return
	}

	currentTrend := indicator.TrendType(int(trendTypeArr[i]))
	macdVal := macdArr[i]
	macdSig := macdSigArr[i]
	rsiVal := rsiArr[i]

	if math.IsNaN(macdVal) || math.IsNaN(macdSig) || math.IsNaN(rsiVal) {
		// 지표가 계산불가(NaN)이면 스킵
		return
	}

	closePrice := df.Close[i]
	openPrice := df.Open[i]

	// 포지션 조회 (잔고 확인용)
	coinAmt, krwAmt, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	// ----- (3) 단순 매매 시그널 예시 -----
	shouldBuy := false
	shouldSell := false

	switch currentTrend {
	case indicator.Bullish:
		// 상승장 예시:
		// - MACD > Signal
		// - RSI < 70
		// - 현재봉 양봉(종가 > 시가)
		// 상승장 예시: MACD > Signal && RSI < 70 && 양봉
		if macdVal > macdSig && rsiVal < 70 && closePrice > openPrice {
			shouldBuy = (coinAmt == 0) // 보유코인 없을 때만 매수 시도
		}

	case indicator.Bearish:
		// 하락장 예시: MACD < Signal && RSI > 70
		if macdVal < macdSig && rsiVal > 70 {
			shouldSell = (coinAmt > 0) // 보유코인 있을 때만 매도
		}

	case indicator.Sideways:
		// 횡보 예시: RSI < 30 매수, RSI > 70 매도
		if rsiVal < 30 {
			shouldBuy = (coinAmt == 0)
		} else if rsiVal > 70 {
			shouldSell = (coinAmt > 0)
		}
	}

	// 매수 / 매도 신호가 발생하면, order_feed로 주문을 Publish
	if shouldBuy && krwAmt >= 5000 {
		// 예: "시장가 매수, KRW 전액 사용" (Upbit가정: 시장가 매수 시 'price' param에 KRW금액)
		buyOrder := model.Order{
			Pair:       df.Pair,
			Side:       model.SideTypeBuy,
			Type:       model.OrderTypePrice, // upbit 시장가 매수
			Price:      krwAmt,               // KRW 전액
			Quantity:   0,                    // 수량은 빈값
			ExchangeID: "",                   // 나중에 채워질 수 있음
		}
		s.orderFeed.Publish(buyOrder)
		log.Infof("[PSHStrategy] 매수신호 -> OrderFeed.Publish(BUY %.2fKRW)", krwAmt)
	}

	if shouldSell && coinAmt > 0 {
		// 예: "시장가 매도, 보유코인 전량"
		sellOrder := model.Order{
			Pair:       df.Pair,
			Side:       model.SideTypeSell,
			Type:       model.OrderTypeMarket, // upbit 시장가 매도
			Quantity:   coinAmt,
			Price:      0,
			ExchangeID: "",
		}
		s.orderFeed.Publish(sellOrder)
		log.Infof("[PSHStrategy] 매도신호 -> OrderFeed.Publish(SELL %.8f)", coinAmt)
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
