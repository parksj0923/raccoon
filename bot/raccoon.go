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

type Raccoon struct {
	exchange           interfaces.Exchange         // 예: Upbit
	dataFeedSub        *feed.DataFeedSubscription  // 실시간 캔들 구독
	orderFeedSub       *feed.OrderFeedSubscription // 주문 신호 발행/구독
	strat              interfaces.Strategy         // 실제 트레이딩 전략
	strategyController *strategy.Controller        // StrategyController
	webServ            *webserver.WebServer        // 차트 그리기 위한 웹서버
	notifier           interfaces.Notifier
}

func NewRaccoon(apiKey, secretKey string, pairs []string) (*Raccoon, error) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		return nil, fmt.Errorf("failed to create Upbit exchange: %w", err)
	}

	dataFeedSub := feed.NewDataFeed(upbit)

	orderFeedSub := feed.NewOrderFeed()

	strat := strategy.NewImprovedPSHStrategy(orderFeedSub)
	ctrl := strategy.NewStrategyController(pairs[0], strat, upbit)

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
		start := end.Add(-dur * time.Duration(warmup))
		log.Infof("[Preload] from=%v to=%v warmup=%d timeframe=%s", start, end, warmup, timeframe)

		candles, err := r.exchange.CandlesByPeriod(pair, timeframe, start, end)
		if err != nil {
			log.Errorf("failed to load warmup candles: %v", err)
		} else {
			r.dataFeedSub.Preload(pair, timeframe, candles)
			log.Infof("[Preload] loaded %d warmup candles for %s-%s", len(candles), pair, timeframe)
		}
	}
}

func (r *Raccoon) Start() {
	log.Infof("Raccoon starting...")

	r.SetupSubscriptions()

	//TODO private websocket 열어서 매매결과도 받아와야함
	r.exchange.Start()

	r.dataFeedSub.Start(false)

	r.orderFeedSub.Start()

	r.strategyController.Start()

	go func() {
		err := r.webServ.Start(":3030")
		if err != nil {
			log.Error("Webserver error:", err)
		}
	}()

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

	strategyInfo := fmt.Sprintf("종목: %s\nTimeframe: %s\nStrategy: %s\n",
		r.strategyController.Dataframe.Pair,
		r.strat.Timeframe(),
		r.strat.GetName())

	if r.notifier != nil {
		notifyMsg := fmt.Sprintf("Raccoon started successfully.\n%s\n%s", accountInfoMsg, strategyInfo)
		if err := r.notifier.SendNotification(notifyMsg); err != nil {
			log.Errorf("Start notification error: %v", err)
		}
	}

	log.Infof("Raccoon started.")
}

func (r *Raccoon) Stop() {
	log.Infof("Raccoon stopping...")

	r.dataFeedSub.Stop()

	r.orderFeedSub.Stop()

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
