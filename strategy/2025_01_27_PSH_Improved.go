package strategy

import (
	"raccoon/feed"
	"raccoon/indicator"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

const (
	defaultTradeFraction = 0.5

	stopLossPercent   = 0.08
	takeProfitPercent = 0.4

	rsiOverboughtThreshold = 70.0
	rsiOversoldThreshold   = 30.0

	adxTrendThreshold = 25.0

	minimumKRW = 5000.0
)

type ImprovedPSHStrategy struct {
	orderFeed     *feed.OrderFeedSubscription
	tradeFraction float64
}

func NewImprovedPSHStrategy(orderFeed *feed.OrderFeedSubscription, tradeFraction ...float64) *ImprovedPSHStrategy {
	if len(tradeFraction) == 0 {
		return &ImprovedPSHStrategy{
			orderFeed:     orderFeed,
			tradeFraction: defaultTradeFraction,
		}
	}
	return &ImprovedPSHStrategy{
		orderFeed:     orderFeed,
		tradeFraction: tradeFraction[0],
	}
}

func (s *ImprovedPSHStrategy) GetName() string {
	return "PSH_Improved"
}

// Timeframe : 일봉 기준
func (s *ImprovedPSHStrategy) Timeframe() string {
	return "5m"
}

// WarmupPeriod : 월별 추세 분석 등 지표 계산에 필요한 최소 봉 수
func (s *ImprovedPSHStrategy) WarmupPeriod() int {
	return 80
}

// Indicators : 각 봉 완료 시(또는 백테스트 루프 내) 지표 계산.
// 여기서는 DetectMonthlyTrendsDF를 호출해 월별 추세를 구한 뒤, df.Metadata에 저장하고,
// 추가로 EMA, RSI, BollingerBands 등도 예시로 계산해서 저장합니다.
func (s *ImprovedPSHStrategy) Indicators(df *model.Dataframe) []indicator.ChartIndicator {
	n := len(df.Close)
	if n == 0 {
		return nil
	}

	trendArr := indicator.DetectMonthlyTrendsDF(df, 10.0) // threshold=10%
	floatTrend := make([]float64, n)
	for i := 0; i < n; i++ {
		floatTrend[i] = float64(trendArr[i])
	}
	df.Metadata["trend"] = floatTrend

	emaShort := indicator.EMA(df.Close, 10)
	emaLong := indicator.EMA(df.Close, 30)
	rsiSeries := indicator.RSI(df.Close, 14)
	bbUp, bbMid, bbLow := indicator.BB(df.Close, 20, 2.0, indicator.TypeSMA)
	macd, macdSignal, macdHist := indicator.MACD(df.Close, 12, 26, 9)
	stochK, stochD := indicator.Stoch(df.High, df.Low, df.Close, 14, 3, indicator.TypeSMA, 3, indicator.TypeSMA)
	adxSeries := indicator.ADX(df.High, df.Low, df.Close, 14)
	atrSeries := indicator.ATR(df.High, df.Low, df.Close, 14)
	obvSeries := indicator.OBV(df.Close, df.Volume)
	mfiSeries := indicator.MFI(df.High, df.Low, df.Close, df.Volume, 14)
	cciSeries := indicator.CCI(df.High, df.Low, df.Close, 20)

	williamsRSeries := indicator.WilliamsR(df.High, df.Low, df.Close, 14)
	stochRSI_K, stochRSI_D := indicator.StochRSI(df.Close, 14, 3, 3, indicator.TypeSMA)

	df.Metadata["shortMA"] = emaShort
	df.Metadata["longMA"] = emaLong
	df.Metadata["rsi"] = rsiSeries
	df.Metadata["bb_mid"] = bbMid
	df.Metadata["bb_up"] = bbUp
	df.Metadata["bb_low"] = bbLow
	df.Metadata["macd"] = macd
	df.Metadata["macdSignal"] = macdSignal
	df.Metadata["macdHist"] = macdHist
	df.Metadata["stochK"] = stochK
	df.Metadata["stochD"] = stochD
	df.Metadata["adx"] = adxSeries
	df.Metadata["atr"] = atrSeries
	df.Metadata["obv"] = obvSeries
	df.Metadata["mfi"] = mfiSeries
	df.Metadata["cci"] = cciSeries
	df.Metadata["williamsR"] = williamsRSeries
	df.Metadata["stochRSI_K"] = stochRSI_K
	df.Metadata["stochRSI_D"] = stochRSI_D

	indicators := []indicator.ChartIndicator{
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "EMA Short", Color: "red", Style: indicator.StyleLine, Values: emaShort},
				{Name: "EMA Long", Color: "blue", Style: indicator.StyleLine, Values: emaLong},
			},
			Overlay:   true,
			GroupName: "EMA",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "RSI", Color: "purple", Style: indicator.StyleLine, Values: rsiSeries},
			},
			Overlay:   false,
			GroupName: "RSI",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "BB Upper", Color: "gray", Style: indicator.StyleLine, Values: bbUp},
				{Name: "BB Mid", Color: "gray", Style: indicator.StyleLine, Values: bbMid},
				{Name: "BB Lower", Color: "gray", Style: indicator.StyleLine, Values: bbLow},
			},
			Overlay:   true,
			GroupName: "Bollinger",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "MACD", Color: "blue", Style: indicator.StyleLine, Values: macd},
				{Name: "MACD Signal", Color: "red", Style: indicator.StyleLine, Values: macdSignal},
				{Name: "MACD Hist", Color: "green", Style: indicator.StyleHistogram, Values: macdHist},
			},
			Overlay:   false,
			GroupName: "MACD",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "StochK", Color: "red", Style: indicator.StyleLine, Values: stochK},
				{Name: "StochD", Color: "blue", Style: indicator.StyleLine, Values: stochD},
			},
			Overlay:   false,
			GroupName: "Stochastic",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "ADX", Color: "orange", Style: indicator.StyleLine, Values: adxSeries},
				{Name: "ATR", Color: "teal", Style: indicator.StyleLine, Values: atrSeries},
			},
			Overlay:   false,
			GroupName: "ADX/ATR",
			Warmup:    s.WarmupPeriod(),
		},
		{
			Time: df.Time,
			Metrics: []indicator.IndicatorMetric{
				{Name: "OBV", Color: "magenta", Style: indicator.StyleLine, Values: obvSeries},
			},
			Overlay:   false,
			GroupName: "OBV",
			Warmup:    s.WarmupPeriod(),
		},
	}

	return indicators
}

func (s *ImprovedPSHStrategy) OnCandle(df *model.Dataframe, broker interfaces.Broker) {
	i := len(df.Close) - 1
	if i < s.WarmupPeriod()-1 {
		// 아직 지표를 계산하기에 봉 수가 부족하면 매매 스킵
		return
	}

	shortMA, shortOk := df.Metadata["shortMA"]
	longMA, longOk := df.Metadata["longMA"]
	rsiSeries, rsiSeriesOk := df.Metadata["rsi"]
	bbUp, bbUpOk := df.Metadata["bb_up"]
	bbLow, bbLowOk := df.Metadata["bb_low"]
	macd, macdOk := df.Metadata["macd"]
	macdSignal, macdSignalOk := df.Metadata["macdSignal"]
	trendSeries, trendSeriesOk := df.Metadata["trend"]
	adxSeries, adxSeriesOk := df.Metadata["adx"]
	obvSeries, obvSeriesOk := df.Metadata["obv"]
	williamsR, williamsROk := df.Metadata["williamsR"]
	stochRSI_K, stochRSI_KOk := df.Metadata["stochRSI_K"]

	if !shortOk || !longOk || !rsiSeriesOk || !trendSeriesOk || !bbUpOk || !bbLowOk || !macdOk ||
		!macdSignalOk || !adxSeriesOk || !obvSeriesOk || !williamsROk || !stochRSI_KOk {
		log.Warn("[PSHStrategy] 필수 지표가 누락되었습니다.")
		return
	}

	closePrice := df.Close[i]
	openPrice := df.Open[i]

	// 포지션 조회 (잔고 확인용)
	coinAmt, krwAmt, avgBuyPrice, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	// --- 리스크 관리 (손절/익절) 조건 확인 --- //
	// 보유 포지션이 있을 때만 적용
	if coinAmt > 0 {
		if stopLossTriggered(avgBuyPrice, closePrice) {
			sellQuantity := coinAmt * s.tradeFraction
			s.executeSell(df.Pair, sellQuantity)
			log.Infof("[PSHStrategy] 손절 신호: 현재가격 %.2f <= 평균매입가 %.2f (%.2f%% 손실)", closePrice, avgBuyPrice, stopLossPercent*100)
			return
		}
		if takeProfitTriggered(avgBuyPrice, closePrice) {
			sellQuantity := coinAmt * s.tradeFraction
			s.executeSell(df.Pair, sellQuantity)
			log.Infof("[PSHStrategy] 익절 신호: 현재가격 %.2f >= 평균매입가 %.2f (%.2f%% 상승)", closePrice, avgBuyPrice, takeProfitPercent*100)
			return
		}
	}

	strongBuy := false
	strongSell := false
	normalBuy := false
	normalSell := false

	currentTrend := indicator.TrendType(int(trendSeries[i]))
	strongTrend := adxSeries[i] >= adxTrendThreshold

	switch currentTrend {
	case indicator.Bullish:
		if isGoldenCross(shortMA, longMA, i) &&
			isMACDCrossover(macd, macdSignal, i) &&
			rsiSeries[i] < rsiOverboughtThreshold &&
			closePrice > openPrice &&
			isIncreasingVolume(df.Volume, i) &&
			strongTrend {
			normalBuy = true
		}
		if i > 0 && obvSeries[i] > obvSeries[i-1] {
			strongBuy = true
		}
		if isDeathCross(shortMA, longMA, i) ||
			rsiSeries[i] > rsiOverboughtThreshold ||
			isMACDDeathCross(macd, macdSignal, i) {
			normalSell = true
		}
		if isDecreasingHigh(df.High, i) && closePrice < openPrice && rsiSeries[i] > (rsiOversoldThreshold+10) {
			normalSell = true
		}

	case indicator.Bearish:
		if isDeathCross(shortMA, longMA, i) &&
			rsiSeries[i] > rsiOverboughtThreshold &&
			isMACDDeathCross(macd, macdSignal, i) {
			normalSell = true
		}
		if williamsR[i] > -20 || stochRSI_K[i] > 80 {
			strongSell = true
		}
		if isGoldenCross(shortMA, longMA, i) &&
			rsiSeries[i] < rsiOversoldThreshold &&
			closePrice <= bbLow[i] {
			normalBuy = true
		}
		if isIncreasingLow(df.Low, i) && closePrice > openPrice && rsiSeries[i] < rsiOversoldThreshold {
			normalBuy = true
		}

	case indicator.Sideways:
		if rsiSeries[i] < rsiOversoldThreshold && closePrice <= (bbLow[i]+(bbUp[i]-bbLow[i])/2) {
			normalBuy = true
			if williamsR[i] < -80 && stochRSI_K[i] < 20 {
				strongBuy = true
			}
		}
		if rsiSeries[i] > rsiOverboughtThreshold && closePrice >= (bbUp[i]-(bbUp[i]-bbLow[i])/2) {
			normalSell = true
			if williamsR[i] > -20 && stochRSI_K[i] > 80 {
				strongSell = true
			}
		}
	}

	if strongSell {
		if coinAmt > 0 {
			s.executeSell(df.Pair, coinAmt) // 전량 매도
			log.Infof("[PSHStrategy] %s 강한 매도신호 -> 전량 매도", df.Pair)
		}
	} else if normalSell {
		if coinAmt > 0 {
			sellQuantity := coinAmt * s.tradeFraction
			s.executeSell(df.Pair, sellQuantity)
			log.Infof("[PSHStrategy] %s 매도신호 -> OrderFeed.Publish(SELL %.8f)", df.Pair, sellQuantity)
		}
	}

	if strongBuy {
		if krwAmt >= minimumKRW {
			s.executeBuy(df.Pair, krwAmt)
			log.Infof("[PSHStrategy] %s 강한 매수신호 -> 전량 매수", df.Pair)
		} else {
			log.Infof("[PSHStrategy] %s 강한 매수신호 발생했으나 잔고 부족 (KRW: %.2f)", df.Pair, krwAmt)
		}
	} else if normalBuy {
		if krwAmt >= minimumKRW {
			buyAmount := krwAmt * s.tradeFraction
			s.executeBuy(df.Pair, buyAmount)
			log.Infof("[PSHStrategy] %s 매수신호 -> OrderFeed.Publish(BUY %.2fKRW)", df.Pair, buyAmount)
		} else {
			log.Infof("[PSHStrategy] %s 매수신호 발생했으나 잔고 부족 (KRW: %.2f)", df.Pair, krwAmt)
		}
	}
}

// --- 주문 실행 헬퍼 --- //
func (s *ImprovedPSHStrategy) executeBuy(pair string, amount float64) {
	buyOrder := model.Order{
		Pair:     pair,
		Side:     model.SideTypeBuy,
		Type:     model.OrderTypePrice, // Upbit의 경우 시장가 매수 시 'price' 필드에 금액 지정
		Price:    amount,
		Quantity: 0,
		ID:       0,
	}
	s.orderFeed.Publish(buyOrder)
}

func (s *ImprovedPSHStrategy) executeSell(pair string, quantity float64) {
	sellOrder := model.Order{
		Pair:       pair,
		Side:       model.SideTypeSell,
		Type:       model.OrderTypeMarket, // 시장가 매도
		Quantity:   quantity,
		Price:      0,
		ExchangeID: "",
	}
	s.orderFeed.Publish(sellOrder)
}

// MA 골든크로스: 단기 MA가 장기 MA를 상향 돌파하는지
func isGoldenCross(shortMA, longMA []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return shortMA[idx] > longMA[idx] && shortMA[idx-1] <= longMA[idx-1]
}

// MA 데드크로스: 단기 MA가 장기 MA를 하향 돌파하는지
func isDeathCross(shortMA, longMA []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return shortMA[idx] < longMA[idx] && shortMA[idx-1] >= longMA[idx-1]
}

// 거래량 증가 여부
func isIncreasingVolume(vol []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return vol[idx] > vol[idx-1]
}

// 최근 3봉의 저가 상승 여부
func isIncreasingLow(low []float64, idx int) bool {
	if idx < 2 {
		return false
	}
	return low[idx-2] < low[idx-1] && low[idx-1] < low[idx]
}

// 최근 3봉의 고가 하락 여부
func isDecreasingHigh(high []float64, idx int) bool {
	if idx < 2 {
		return false
	}
	return high[idx-2] > high[idx-1] && high[idx-1] > high[idx]
}

// MACD 크로스오버: MACD 라인이 Signal 라인을 상향 돌파하는지
func isMACDCrossover(macd, signal []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return macd[idx] > signal[idx] && macd[idx-1] <= signal[idx-1]
}

// MACD 데드크로스: MACD 라인이 Signal 라인을 하향 돌파하는지
func isMACDDeathCross(macd, signal []float64, idx int) bool {
	if idx < 1 {
		return false
	}
	return macd[idx] < signal[idx] && macd[idx-1] >= signal[idx-1]
}

// 손절 조건: 현재가격이 평균매입가 대비 일정 비율 이하인 경우
func stopLossTriggered(avgBuyPrice, currentPrice float64) bool {
	return currentPrice <= avgBuyPrice*(1-stopLossPercent)
}

// 익절 조건: 현재가격이 평균매입가 대비 일정 비율 이상인 경우
func takeProfitTriggered(avgBuyPrice, currentPrice float64) bool {
	return currentPrice >= avgBuyPrice*(1+takeProfitPercent)
}
