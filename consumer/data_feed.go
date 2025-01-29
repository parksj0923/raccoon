package consumer

import (
	"raccoon/chartview"
	"raccoon/model"
	"raccoon/strategy"
)

type DataFeedConsumer struct {
	strategyController *strategy.Controller
}

func NewDataFeedConsumer(controller *strategy.Controller) *DataFeedConsumer {
	return &DataFeedConsumer{
		strategyController: controller,
	}
}

func (c *DataFeedConsumer) OnCandle(candle model.Candle) {
	c.strategyController.OnCandle(candle)

	chartview.GlobalChartData.AppendCandle(candle)
}
