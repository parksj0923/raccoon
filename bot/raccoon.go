package bot

import (
	"fmt"
	"raccoon/chartview"
	"raccoon/consumer"
	"raccoon/exchange"
	"raccoon/feed"
	"raccoon/interfaces"
	"raccoon/strategy"
	"raccoon/utils/log"
	"raccoon/utils/tools"
	"time"
)

// Raccoon : 전체 트레이딩 봇을 관리하는 구조체
type Raccoon struct {
	exchange           interfaces.Exchange         // 예: Upbit
	dataFeedSub        *feed.DataFeedSubscription  // 실시간 캔들 구독
	orderFeedSub       *feed.OrderFeedSubscription // 주문 신호 발행/구독
	strat              interfaces.Strategy         // 실제 트레이딩 전략
	strategyController *strategy.Controller        // StrategyController

	dataFeedConsumer  *consumer.DataFeedConsumer
	orderFeedConsumer *consumer.OrderFeedConsumer
}

// NewRaccoon : Raccoon 인스턴스 생성
//   - apiKey, secretKey : 업비트 API 키
//   - pairs : 관심 종목 예) ["KRW-BTC"]
func NewRaccoon(apiKey, secretKey string, pairs []string) (*Raccoon, error) {
	// 1) Upbit (Exchange + Broker)
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		return nil, fmt.Errorf("failed to create Upbit exchange: %w", err)
	}

	// 2) DataFeedSubscription (캔들 구독)
	dataFeedSub := feed.NewDataFeed(upbit)

	// 3) OrderFeedSubscription (주문 처리)
	orderFeedSub := feed.NewOrderFeed()

	// 4) Strategy : 예시로 PSHStrategy
	strat := strategy.NewPSHStrategy(orderFeedSub)
	ctrl := strategy.NewStrategyController(pairs[0], strat, upbit)

	return &Raccoon{
		exchange:           upbit,
		dataFeedSub:        dataFeedSub,
		orderFeedSub:       orderFeedSub,
		strat:              strat,
		strategyController: ctrl,
	}, nil
}

// SetupSubscriptions : DataFeed, OrderFeed 구독 설정
//   - candle 구독 → controller.OnCandle 호출
//   - 주문 구독(OrderManager) → 실제 Broker 주문 실행
func (r *Raccoon) SetupSubscriptions() {

	pair := r.strategyController.Dataframe.Pair
	timeframe := r.strat.Timeframe()
	warmup := r.strat.WarmupPeriod()

	dataFeedCosumer := consumer.NewDataFeedConsumer(r.strategyController)
	r.dataFeedSub.Subscribe(
		pair,
		timeframe,
		dataFeedCosumer.OnCandle,
		true, // onCandleClose = true → 완성된 봉만 콜백
	)

	orderFeedConsumer := consumer.NewOrderFeedConsumer(r.exchange)
	r.orderFeedSub.Subscribe(pair, orderFeedConsumer.OnOrder)

	// -------------------------------------------
	// 미리 WarmupPeriod만큼의 과거캔들 Preload
	// -------------------------------------------
	dur, err := tools.ParseTimeframeToDuration(timeframe)
	if err != nil {
		log.Warnf("Cannot parse timeframe: %v => skip preload", err)
	} else {
		KSTLoc, _ := time.LoadLocation("Asia/Seoul")
		end := time.Now().In(KSTLoc)
		start := end.Add(-dur * time.Duration(warmup)) // ex) 60일전(1d*60)
		log.Infof("[Preload] from=%v to=%v warmup=%d timeframe=%s", start, end, warmup, timeframe)

		candles, err := r.exchange.CandlesByPeriod(pair, timeframe, start, end)
		if err != nil {
			log.Errorf("failed to load warmup candles: %v", err)
		} else {
			// Preload (주어진 candles를 구독자에게 일괄 전달)
			r.dataFeedSub.Preload(pair, timeframe, candles)
			log.Infof("[Preload] loaded %d warmup candles for %s-%s", len(candles), pair, timeframe)
		}
	}
}

// Start : Raccoon 트레이딩 봇 시작
//   - Upbit.WS Start
//   - dataFeedSub.Start
//   - orderFeedSub.Start
//   - controller.Start
func (r *Raccoon) Start() {
	log.Infof("Raccoon starting...")

	account, err := r.exchange.Account()
	if err != nil {
		log.Errorf("Failed to fetch account info: %v", err)
	} else {
		// 원하는 양식에 맞게 출력
		// account.Balances 내역을 순회하며 각각의 정보도 로깅할 수 있음
		log.Infof("=== [Account Info] ===")
		for _, b := range account.Balances {
			log.Infof("Currency=%s, balance=%.4f, locked=%.4f, avgBuyPrice=%.4f",
				b.Currency, b.Balance, b.Locked, b.AvgBuyPrice)
		}
	}

	// 1) Upbit websocket 시작
	r.exchange.Start()

	go chartview.StartChartServer(":8080")
	log.Infof("Raccoon started. Open http://localhost:8080/chart to see chart!")

	// 2) DataFeedSubscription -> Start
	//    -> Websocket 수신된 Candle -> 구독자에 전달(= controller.OnCandle)
	r.dataFeedSub.Start(false)

	// 3) OrderFeedSubscription -> Start
	//    -> Publish된 주문 -> 구독자(익명 함수) 처리
	r.orderFeedSub.Start()

	// 4) StrategyController 시작(이제부터 OnCandle 시 strategy.OnCandle 도 수행)
	r.strategyController.Start()

	log.Infof("Raccoon started.")
}

// Stop : Raccoon 트레이딩 봇 종료
//   - dataFeedSub.Stop
//   - orderFeedSub.Stop
//   - exchange.Stop
func (r *Raccoon) Stop() {
	log.Infof("Raccoon stopping...")

	// 1) DataFeedSubscription 정지
	r.dataFeedSub.Stop()

	// 2) OrderFeedSubscription 정지
	r.orderFeedSub.Stop()

	// 3) Upbit 정지(WS close)
	r.exchange.Stop()

	log.Infof("Raccoon stopped.")
}
