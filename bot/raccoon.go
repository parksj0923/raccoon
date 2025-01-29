package bot

import (
	"fmt"
	"raccoon/consumer"
	"raccoon/exchange"
	"raccoon/feed"
	"raccoon/interfaces"
	"raccoon/strategy"
	"raccoon/utils/log"
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

	dataFeedCosumer := consumer.NewDataFeedConsumer(r.strategyController)
	orderFeedConsumer := consumer.NewOrderFeedConsumer(r.exchange)
	// 예: "KRW-BTC"와 Strategy.Timeframe()을 이용해 구독
	pair := r.strategyController.Dataframe.Pair
	timeframe := r.strat.Timeframe()

	r.dataFeedSub.Subscribe(
		pair,
		timeframe,
		dataFeedCosumer.OnCandle,
		true, // onCandleClose = true → 완성된 봉만 콜백
	)

	r.orderFeedSub.Subscribe(pair, orderFeedConsumer.OnMarketOrder)
}

// Start : Raccoon 트레이딩 봇 시작
//   - Upbit.WS Start
//   - dataFeedSub.Start
//   - orderFeedSub.Start
//   - controller.Start
func (r *Raccoon) Start() {
	log.Infof("Raccoon starting...")

	// 1) Upbit websocket 시작
	r.exchange.Start()

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
