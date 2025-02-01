package strategy

import (
	"raccoon/indicator"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
	"raccoon/webserver"
	"time"
)

type Controller struct {
	Strategy  interfaces.Strategy
	Dataframe *model.Dataframe
	Broker    interfaces.Broker
	WebServer interfaces.WebServer
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
		chartIndics := c.Strategy.Indicators(&sample)

		if c.started {
			c.Strategy.OnCandle(&sample, c.Broker)

			results, timestamp := makeChartIndicators(&sample, chartIndics)
			if c.WebServer != nil && len(results) > 0 {
				c.WebServer.OnIndicators(timestamp, results)
			}
		}
	}
}

func (c *Controller) OnPartialCandle(candle model.Candle) {
	//TODO 파셜 받았을떄 해야함
}

func makeChartIndicators(sample *model.Dataframe, chartIndics []indicator.ChartIndicator) ([]webserver.IndicatorValue, time.Time) {
	lastIndex := sample.Close.Length() - 1
	timestamp := sample.Time[lastIndex] // 마지막 봉 시각

	// IndicatorValue : "지표 이름" + "지표 값"
	var results []webserver.IndicatorValue

	for _, ci := range chartIndics {
		// ci.Metrics : 여러 라인(예: MACD, MACD Signal, MACD Hist)
		for _, metric := range ci.Metrics {
			// metric.Values : model.Series[float64], 길이= dfSample.Close.Length()
			if metric.Values.Length() > lastIndex {
				val := metric.Values[lastIndex]
				results = append(results, webserver.IndicatorValue{
					Name:  metric.Name,
					Value: val,
				})
			}
		}
	}
	return results, timestamp
}
