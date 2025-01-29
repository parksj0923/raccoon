package strategy

import (
	"raccoon/chartview"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

type Controller struct {
	Strategy  interfaces.Strategy
	Dataframe *model.Dataframe
	Broker    interfaces.Broker
	started   bool
}

func NewStrategyController(pair string, strategy interfaces.Strategy, broker interfaces.Broker) *Controller {
	dataframe := &model.Dataframe{
		Pair:     pair,
		Metadata: make(map[string]model.Series[float64]),
	}
	return &Controller{
		Strategy:  strategy,
		Dataframe: dataframe,
		Broker:    broker,
	}
}

func (c *Controller) Start() {
	c.started = true
}

func (c *Controller) updateDataFrame(candle model.Candle) {
	if len(c.Dataframe.Time) > 0 && candle.Time.Equal(c.Dataframe.Time[len(c.Dataframe.Time)-1]) {
		last := len(c.Dataframe.Time) - 1
		c.Dataframe.Close[last] = candle.Close
		c.Dataframe.Open[last] = candle.Open
		c.Dataframe.High[last] = candle.High
		c.Dataframe.Low[last] = candle.Low
		c.Dataframe.Volume[last] = candle.Volume
		c.Dataframe.Time[last] = candle.Time
		for k, v := range candle.Metadata {
			c.Dataframe.Metadata[k][last] = v
		}
	} else {
		c.Dataframe.Close = append(c.Dataframe.Close, candle.Close)
		c.Dataframe.Open = append(c.Dataframe.Open, candle.Open)
		c.Dataframe.High = append(c.Dataframe.High, candle.High)
		c.Dataframe.Low = append(c.Dataframe.Low, candle.Low)
		c.Dataframe.Volume = append(c.Dataframe.Volume, candle.Volume)
		c.Dataframe.Time = append(c.Dataframe.Time, candle.Time)
		c.Dataframe.LastUpdate = candle.Time
		for k, v := range candle.Metadata {
			c.Dataframe.Metadata[k] = append(c.Dataframe.Metadata[k], v)
		}
	}
}

func (c *Controller) OnCandle(candle model.Candle) {
	if len(c.Dataframe.Time) > 0 && candle.Time.Before(c.Dataframe.Time[len(c.Dataframe.Time)-1]) {
		log.Errorf("late candle received: %#v", candle)
		return
	}

	c.updateDataFrame(candle)

	if len(c.Dataframe.Close) >= c.Strategy.WarmupPeriod() {
		sample := c.Dataframe.Sample(c.Strategy.WarmupPeriod())
		c.Strategy.Indicators(&sample)

		macdArr := sample.Metadata["macd"]
		macdSigArr := sample.Metadata["macdSignal"]
		macdHistArr := sample.Metadata["macdHist"]
		rsiArr := sample.Metadata["rsi14"]

		chartview.GlobalChartData.UpdateIndicators(rsiArr, macdArr, macdSigArr, macdHistArr)

		if c.started {
			c.Strategy.OnCandle(&sample, c.Broker)
		}
	}
}

func (c *Controller) OnPartialCandle(candle model.Candle) {
	//TODO 파셜 받았을떄 해야함
}
