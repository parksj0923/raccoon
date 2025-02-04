package strategy

import (
	"raccoon/indicator"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
	"raccoon/utils/tools"
)

type trailing struct {
	trailingStop map[string]*tools.TrailingStop
	scheduler    map[string]*tools.Scheduler
}

func NewTrailing(pairs []string) interfaces.HighFrequencyStrategy {
	strategy := &trailing{
		trailingStop: make(map[string]*tools.TrailingStop),
		scheduler:    make(map[string]*tools.Scheduler),
	}

	for _, pair := range pairs {
		strategy.trailingStop[pair] = tools.NewTrailingStop()
		strategy.scheduler[pair] = tools.NewScheduler(pair)
	}

	return strategy
}
func (t trailing) GetName() string {
	return "TrailingStop"
}

func (t trailing) Timeframe() string {
	return "4h"
}

func (t trailing) WarmupPeriod() int {
	return 21
}

func (t trailing) Indicators(df *model.Dataframe) []indicator.ChartIndicator {
	df.Metadata["ema_fast"] = indicator.EMA(df.Close, 8)
	df.Metadata["sma_slow"] = indicator.SMA(df.Close, 21)

	return nil
}

func (t trailing) OnCandle(df *model.Dataframe, broker interfaces.Broker) {
	asset, quote, _, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	if quote > 10.0 && // enough cash?
		asset*df.Close.Last(0) < 10 && // without position yet
		df.Metadata["ema_fast"].Crossover(df.Metadata["sma_slow"]) {
		_, err = broker.CreateOrderMarket(model.SideTypeBuy, df.Pair, quote)
		if err != nil {
			log.Error(err)
			return
		}

		t.trailingStop[df.Pair].Start(df.Close.Last(0), df.Low.Last(0))

		return
	}
}

func (t trailing) OnPartialCandle(df *model.Dataframe, broker interfaces.Broker) {
	if trailing := t.trailingStop[df.Pair]; trailing != nil && trailing.Update(df.Close.Last(0)) {
		asset, _, _, err := broker.Position(df.Pair)
		if err != nil {
			log.Error(err)
			return
		}

		if asset > 0 {
			_, err = broker.CreateOrderMarket(model.SideTypeSell, df.Pair, asset)
			if err != nil {
				log.Error(err)
				return
			}
			trailing.Stop()
		}
	}
}
