package main

import (
	"fmt"
	"os"
	"time"

	"raccoon/exchange"
	"raccoon/feed"
	"raccoon/model"
	"raccoon/strategy"
	rlog "raccoon/utils/log"
)

var currentPrice float64 = 0.0

func processOrder(order model.Order, broker *exchange.BacktestBroker) {
	if order.Side == model.SideTypeBuy {
		// 매수 주문: order.Price 필드에 지정된 금액(KRW)로 매수
		qty := order.Price / currentPrice
		if broker.KRW >= order.Price {
			// 평균 매입가 재계산: (기존매수금액 + 이번매수금액) / (기존 수량 + 이번 수량)
			totalCost := broker.AvgBuyPrice*broker.Coin + currentPrice*qty
			broker.Coin += qty
			broker.KRW -= order.Price
			broker.AvgBuyPrice = totalCost / broker.Coin
			rlog.Infof("Executed BUY: Spent %.2f KRW to buy %.6f coins at price %.2f", order.Price, qty, currentPrice)
		} else {
			rlog.Warnf("Not enough KRW to execute BUY. Available: %.2f, Required: %.2f", broker.KRW, order.Price)
		}
	} else if order.Side == model.SideTypeSell {
		// 매도 주문: order.Quantity 필드에 지정된 코인 수량으로 매도
		if broker.Coin >= order.Quantity {
			proceeds := order.Quantity * currentPrice
			broker.Coin -= order.Quantity
			broker.KRW += proceeds
			rlog.Infof("Executed SELL: Sold %.6f coins at price %.2f for %.2f KRW", order.Quantity, currentPrice, proceeds)
		} else {
			rlog.Warnf("Not enough coin to execute SELL. Available: %.6f, Required: %.6f", broker.Coin, order.Quantity)
		}
	}
}

func main() {
	pair := "KRW-XRP"
	timeframe := "1m"
	KSTloc, _ := time.LoadLocation("Asia/Seoul")
	start := time.Date(2025, time.January, 01, 00, 00, 0, 0, KSTloc)
	end := time.Date(2025, time.February, 3, 00, 00, 0, 0, KSTloc)

	apiKey := os.Getenv("UPBIT_ACCESS_KEY")
	secretKey := os.Getenv("UPBIT_SECRET_KEY")
	upbit, err := exchange.NewUpbit(apiKey, secretKey, []string{pair})
	if err != nil {
		rlog.Errorf("Failed to create exchange instance: %v", err)
	}

	candles, err := upbit.CandlesByPeriod(pair, timeframe, start, end)
	if err != nil {
		rlog.Errorf("Failed to load historical candles: %v", err)
	}
	rlog.Infof("Loaded %d candles for %s from %v to %v", len(candles), pair, start, end)

	broker := exchange.NewBackTestBroker(pair, 100000000.0)

	orderFeed := feed.NewOrderFeed()
	strat := strategy.NewImprovedPSHStrategy(orderFeed)

	ctrl := strategy.NewStrategyController(pair, strat, broker)

	orderFeed.Subscribe(pair, func(order model.Order) {
		processOrder(order, broker)
	})
	orderFeed.Start()
	ctrl.Start()

	for _, candle := range candles {
		currentPrice = candle.Close
		ctrl.OnCandle(candle)
	}

	finalValue := broker.KRW + broker.Coin*currentPrice
	fmt.Printf("Backtest completed.\nFinal KRW balance: %.2f\nFinal Coin holdings: %.6f\nTotal Portfolio Value: %.2f KRW\n",
		broker.KRW, broker.Coin, finalValue)
}
