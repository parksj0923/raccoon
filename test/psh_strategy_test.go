package test

import (
	"raccoon/utils/log"
	"testing"
	"time"

	"raccoon/consumer"
	"raccoon/feed"
	"raccoon/mocks"
	"raccoon/model"
	"raccoon/strategy"
)

// TestPSHStrategy_Integration 는
// PSHStrategy.OnCandle() -> orderFeed.Publish -> OrderFeedConsumerBroker.OnOrder -> mockExchange.CreateOrderMarket
// 이렇게 전체 플로우가 도는지 통합 테스트합니다.
func TestPSHStrategy_Integration(t *testing.T) {
	// 1) Mock Exchange 준비
	mockEx := &mocks.MockExchange{
		MockKrw: 50000, // 5만원 잔고
	}

	// 2) OrderFeedSubscription + Consumer
	orderFeedSub := feed.NewOrderFeed()
	ofc := consumer.NewOrderFeedConsumerBroker(mockEx)

	// pair="KRW-DOGE"로 등록해야 Publish(pair="KRW-DOGE")가 콜백으로 간다
	orderFeedSub.Subscribe("KRW-DOGE", ofc.OnOrder)

	// 3) PSHStrategy 생성 (이 때 orderFeedSub를 주입)
	s := strategy.NewPSHStrategy(orderFeedSub)

	// 4) 가짜 DataFrame: “매수 조건을 만족하도록”
	//    가령 shortMA>longMA, rsi<70, 그리고 양봉 등
	df := &model.Dataframe{
		Pair:   "KRW-DOGE",
		Time:   []time.Time{time.Now().Add(-time.Minute), time.Now()},
		Open:   []float64{100.0, 101.0},
		High:   []float64{101.0, 102.0},
		Low:    []float64{99.0, 99.0},
		Close:  []float64{101.0, 102.0}, // 양봉
		Volume: []float64{10, 20},       // 거래량 증가
		Metadata: map[string]model.Series[float64]{
			"shortMA": {99, 101},
			"longMA":  {101, 100}, // 이제 단기가 장기 돌파
			"rsi":     {60, 65},   // 70 미만 => 매수 신호
		},
	}

	// PSHStrategy는 OnCandle에서 broker.Position()을 확인하므로, mockEx를 broker로 넣어준다
	// (단, WarmupPeriod()=80보다 캔들이 적어서 스킵될 수도 있으니 유의)
	// 여기서는 테스트 편의상 WarmupPeriod를 임시로 낮추거나, df 길이를 80개 이상으로 세팅하세요.
	// 혹은 Strategy 코드에서 "if i < WarmupPeriod() then return" 을 잠시 주석 처리해도 됨.

	// 5) OnCandle() 호출
	s.OnCandle(df, mockEx)

	// 6) OrderFeedConsumerBroker는 Publish된 주문을 처리 -> mockEx.CreateOrderMarket
	//    비동기로 처리한다면 잠시 time.Sleep(100*time.Millisecond) 정도 기다려도 되고,
	//    기본적으로 feed.OrderFeed에 버퍼가 있으면 바로 호출될 겁니다.

	// 7) 검증
	if mockEx.CreateOrderMarketCount == 0 {
		t.Fatalf("PSHStrategy did not generate a market order via MockExchange")
	}
	// 어떤 주문이었는지
	lastOrder := mockEx.LastCreatedOrder
	if lastOrder.Side != model.SideTypeBuy {
		t.Errorf("Wanted buy, got side=%v", lastOrder.Side)
	}
	if lastOrder.Quantity < 1 {
		// 예: 5만원치 매수면 quantity=50000(Upbit 시장가매수=price param)
		t.Errorf("Wanted quantity=50000(krw amount), got %f", lastOrder.Quantity)
	}
}

// TestPSHStrategy_Flow_AfterPreloadAndNewData
//  1. Preload로 80개 봉 => WarmupPeriod(=80) 충족
//  2. dataFeed.Start() 후, 새 1개 봉을 dataFeed.DataFeeds[key].Data 채널에 직접 흘려 넣음(실시간 WS 시뮬레이션)
//  3. PSHStrategy가 이 81번째 봉을 보고 매매 신호를 낼 수 있는지 확인
func TestPSHStrategy_Flow_AfterPreloadAndNewData(t *testing.T) {
	// 1) Mock Broker (잔고 10만원 예시)
	mockEx := &mocks.MockExchange{
		MockKrw: 100000,
	}

	// 2) OrderFeedSubscription
	orderFeed := feed.NewOrderFeed()
	ofc := consumer.NewOrderFeedConsumerBroker(mockEx)
	orderFeed.Subscribe("KRW-DOGE", ofc.OnOrder)

	// 3) PSHStrategy + Controller
	strat := strategy.NewPSHStrategy(orderFeed)
	ctrl := strategy.NewStrategyController("KRW-DOGE", strat, mockEx)

	// 4) DataFeedSubscription
	dataFeed := feed.NewDataFeed(mockEx)
	dfConsumer := consumer.NewDataFeedConsumerStrategy(ctrl)
	// onCandleClose=true -> Complete=true 봉만 전달
	dataFeed.Subscribe("KRW-DOGE", "1m", dfConsumer.OnCandle, true)

	// -- (A) Preload 80개 봉 -> Warmup 충족만 시켜둔다 --
	var candles []model.Candle
	now := time.Now()
	for i := 0; i < 80; i++ {
		c := model.Candle{
			Pair: "KRW-DOGE",
			Time: now.Add(time.Minute * time.Duration(i)),
			// 시가/종가를 일정 패턴으로 만들어서 후반부에 "단기MA>장기MA" 등 조건을 유도 가능
			Open:     100.0 + float64(i),
			High:     101.0 + float64(i),
			Low:      99.0 + float64(i),
			Close:    100.5 + float64(i),
			Volume:   10.0 + float64(i),
			Complete: true,
		}
		candles = append(candles, c)
	}

	// 마지막 1~2개 봉에서 확실히 "단기>장기"나 "rsi<70"이 나오도록
	// => PSHStrategy에서 매수 조건이 걸릴 수 있음
	// 여기선 간단하게 "나중 봉의 종가를 확 끌어올려" 단기MA가 장기MA보다 높아지도록
	candles[78].Close = 200
	candles[79].Close = 250
	// 조금 변동주고 싶다면 마지막 몇 봉을 150, 200 식으로 올려도 됨
	dataFeed.Preload("KRW-DOGE", "1m", candles)

	// -- (B) Start 구독 루틴
	mockEx.Start()
	dataFeed.Start(false) // Goroutine이 feed.Data[channel]을 수신하기 시작
	orderFeed.Start()
	ctrl.Start()
	time.Sleep(3 * time.Second)

	// Preload만으로도 이미 80봉 → OnCandle 80번 호출
	// 하지만 PSHStrategy가 매매 시그널을 낼 수도 있고, 안 낼 수도 있음
	// (조건에 따라 달라짐)

	// (C) 이제 "새로운 실시간 봉"을 채널에 넣어서 => 전략이 "81번째 봉"으로 처리하도록
	key := "KRW-DOGE_1m"
	dfStruct, ok := dataFeed.DataFeeds[key]
	if !ok {
		t.Fatalf("DataFeed for key=%s not found", key)
	}

	// 81번째 봉
	newCandle := model.Candle{
		Pair:     "KRW-DOGE",
		Time:     now.Add(80 * time.Minute),
		Open:     200,
		High:     210,
		Low:      195,
		Close:    205,
		Volume:   50,
		Complete: true, // onCandleClose=true -> 반드시 true
	}

	// 실제 WebSocket 데이터가 들어온 것처럼 흉내
	dfStruct.Data <- newCandle

	// 약간 대기(비동기로 OnCandle 처리)
	time.Sleep(100 * time.Second)

	// (D) 검증: 새 봉(81번째)에서 매매가 발생했나?
	if mockEx.CreateOrderMarketCount == 0 {
		t.Errorf("Expected a trade after new candle, but got 0. Possibly no buy/sell condition met.")
	} else {
		lastOrd := mockEx.LastCreatedOrder
		log.Infof("New Candle triggered an order => side=%v, quantity=%.2f", lastOrd.Side, lastOrd.Quantity)
	}

	// Stop
	dataFeed.Stop()
	orderFeed.Stop()
}
