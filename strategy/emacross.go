package strategy

import (
	"raccoon/indicator"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

type CrossEMA struct{}

func (e CrossEMA) Timeframe() string {
	return "4h"
}

func (e CrossEMA) WarmupPeriod() int {
	return 22
}

func (e CrossEMA) Indicators(df *model.Dataframe) []indicator.ChartIndicator {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["sma21"] = indicator.SMA(df.Close, 21)

	return []indicator.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "MA's",
			Time:      df.Time,
			Metrics: []indicator.IndicatorMetric{
				{
					Values: df.Metadata["ema8"],
					Name:   "EMA 8",
					Color:  "red",
					Style:  indicator.StyleLine,
				},
				{
					Values: df.Metadata["sma21"],
					Name:   "SMA 21",
					Color:  "blue",
					Style:  indicator.StyleLine,
				},
			},
		},
	}
}

func (e *CrossEMA) OnCandle(df *model.Dataframe, broker interfaces.Broker) {
	closePrice := df.Close.Last(0)

	assetPosition, quotePosition, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	if quotePosition >= 10 && // minimum quote position to trade
		df.Metadata["ema8"].Crossover(df.Metadata["sma21"]) { // trade signal (EMA8 > SMA21)

		amount := quotePosition / closePrice // calculate amount of asset to buy
		_, err := broker.CreateOrderMarket(model.SideTypeBuy, df.Pair, amount)
		if err != nil {
			log.Error(err)
		}

		return
	}

	if assetPosition > 0 &&
		df.Metadata["ema8"].Crossunder(df.Metadata["sma21"]) { // trade signal (EMA8 < SMA21)

		_, err = broker.CreateOrderMarket(model.SideTypeSell, df.Pair, assetPosition)
		if err != nil {
			log.Error(err)
		}
	}
}
