package consumer

import (
	"raccoon/model"
	"raccoon/strategy"
)

type DataFeedConsumerStrategy struct {
	strategyController *strategy.Controller
}

func NewDataFeedConsumerStrategy(controller *strategy.Controller) *DataFeedConsumerStrategy {
	return &DataFeedConsumerStrategy{
		strategyController: controller,
	}
}

func (c *DataFeedConsumerStrategy) OnCandle(candle model.Candle) {
	c.strategyController.OnCandle(candle)
}
