package bot

import (
	"fmt"
	"raccoon/consumer"
	"raccoon/exchange"
	"raccoon/feed"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/strategy"
	"raccoon/utils/log"
	"raccoon/utils/tools"
	"raccoon/webserver"
	"time"
)

// Raccoon : 전체 트레이딩 봇을 관리하는 구조체
type Raccoon struct {
	exchange           interfaces.Exchange         // 예: Upbit
	dataFeedSub        *feed.DataFeedSubscription  // 실시간 캔들 구독
	orderFeedSub       *feed.OrderFeedSubscription // 주문 신호 발행/구독
	strat              interfaces.Strategy         // 실제 트레이딩 전략
	strategyController *strategy.Controller        // StrategyController
	webServ            *webserver.WebServer        // 차트 그리기 위한 웹서버
	notifier           interfaces.Notifier
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

	// 5) Web Server
	webServ := webserver.NewWebServer()

	return &Raccoon{
		exchange:           upbit,
		dataFeedSub:        dataFeedSub,
		orderFeedSub:       orderFeedSub,
		strat:              strat,
		strategyController: ctrl,
		webServ:            webServ,
	}, nil
}

// SetupSubscriptions : DataFeed, OrderFeed 구독 설정
//   - candle 구독 → controller.OnCandle 호출
//   - 주문 구독(OrderManager) → 실제 Broker 주문 실행
func (r *Raccoon) SetupSubscriptions() {

	pair := r.strategyController.Dataframe.Pair
	timeframe := r.strat.Timeframe()
	warmup := r.strat.WarmupPeriod()

	consumerStrategy := consumer.NewDataFeedConsumerStrategy(r.strategyController)
	r.dataFeedSub.Subscribe(
		pair,
		timeframe,
		consumerStrategy.OnCandle,
		true, // onCandleClose = true → 완성된 봉만 콜백
	)

	r.dataFeedSub.Subscribe(
		pair,
		timeframe,
		r.webServ.OnCandle,
		false,
	)

	consumerBroker := consumer.NewOrderFeedConsumerBroker(r.exchange)
	r.orderFeedSub.Subscribe(pair, consumerBroker.OnOrder)

	r.orderFeedSub.Subscribe(pair, r.webServ.OnOrder)

	if r.notifier != nil {
		consumerBroker.AddOrderExecutedCallback(func(order model.Order, err error) {
			r.notifier.OrderNotifier(order, err)
		})
	}

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

	r.SetupSubscriptions()

	account, err := r.exchange.Account()
	var accountInfoMsg string
	if err != nil {
		log.Errorf("Failed to fetch account info: %v", err)
		accountInfoMsg = fmt.Sprintf("Failed to fetch account info: %v", err)
	} else {
		accountInfoMsg = "=== [Account Info] ===\n"
		for _, b := range account.Balances {
			accountInfoMsg += fmt.Sprintf("Currency=%s, balance=%.4f, locked=%.4f, avgBuyPrice=%.4f\n",
				b.Currency, b.Balance, b.Locked, b.AvgBuyPrice)
		}
	}
	log.Infof(accountInfoMsg)

	// 1) Upbit websocket 시작
	//TODO private websocket 열어서 매매결과도 받아와야함
	r.exchange.Start()

	// 2) DataFeedSubscription -> Start
	//    -> Websocket 수신된 Candle -> 구독자에 전달(= controller.OnCandle)
	r.dataFeedSub.Start(false)

	// 3) OrderFeedSubscription -> Start
	//    -> Publish된 주문 -> 구독자(익명 함수) 처리
	r.orderFeedSub.Start()

	// 4) StrategyController 시작(이제부터 OnCandle 시 strategy.OnCandle 도 수행)
	r.strategyController.Start()

	// 5) Web server start
	go func() {
		err := r.webServ.Start(":3030")
		if err != nil {
			log.Error("Webserver error:", err)
		}
	}()

	if r.notifier != nil {
		notifyMsg := fmt.Sprintf("Raccoon started successfully.\n%s", accountInfoMsg)
		if err := r.notifier.SendNotification(notifyMsg); err != nil {
			log.Errorf("Start notification error: %v", err)
		}
	}

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

	account, err := r.exchange.Account()
	var accountInfoMsg string
	if err != nil {
		accountInfoMsg = fmt.Sprintf("Failed to fetch account info: %v", err)
	} else {
		accountInfoMsg = "=== [Account Info] ===\n"
		for _, b := range account.Balances {
			accountInfoMsg += fmt.Sprintf("Currency=%s, balance=%.4f, locked=%.4f, avgBuyPrice=%.4f\n",
				b.Currency, b.Balance, b.Locked, b.AvgBuyPrice)
		}
	}

	r.exchange.Stop()

	if r.notifier != nil {
		notifyMsg := fmt.Sprintf("Raccoon stopped.\n%s", accountInfoMsg)
		if err := r.notifier.SendNotification(notifyMsg); err != nil {
			log.Errorf("Stop notification error: %v", err)
		}
	}

	log.Infof("Raccoon stopped.")
}

func (r *Raccoon) SetNotifier(notifier interfaces.Notifier) {
	r.notifier = notifier
}
