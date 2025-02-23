package interfaces

import (
	"raccoon/indicator"
	"raccoon/model"
	"raccoon/webserver"
	"time"
)

type Exchange interface {
	Broker
	DataFeeder
}

type Broker interface {
	Account() (model.Asset, error)
	Position(pair string) (asset, quote, avgBuyPrice float64, err error)
	OrderChance(pair string) (*model.OrderChance, error)
	Order(pair string, uuidOrIdentifier string, isIdentifier bool) (model.Order, error)
	OpenOrders(pair string, limit int) ([]model.Order, error)
	CreateOrderLimit(side model.SideType, pair string,
		quantity float64, limit float64, tif ...model.TimeInForceType) (model.Order, error)
	CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error)
	CreateOrderBest(side model.SideType, pair string, quantity float64, tif ...model.TimeInForceType) (model.Order, error)
	Cancel(order model.Order, isIdentifier bool) error
}

type DataFeeder interface {
	AssetsInfo(pair string) model.AssetInfo
	LastQuote(pair string) (float64, error)
	CandlesByLimit(pair, period string, limit int) ([]model.Candle, error)
	CandlesByPeriod(pair, period string, start, end time.Time) ([]model.Candle, error)
	CandlesSubscription(pair, timeframe string) (chan model.Candle, chan error)
	Start()
	Stop()
}

type Notifier interface {
	SendNotification(message string) error
	OrderNotifier(order model.Order, err error)
}

type Strategy interface {
	GetName() string
	Timeframe() string
	WarmupPeriod() int
	Indicators(df *model.Dataframe) []indicator.ChartIndicator

	OnCandle(df *model.Dataframe, broker Broker)
}

type HighFrequencyStrategy interface {
	Strategy
	OnPartialCandle(df *model.Dataframe, broker Broker)
}

type WebServer interface {
	OnCandle(candle model.Candle)
	OnOrder(order model.Order)
	OnIndicators(timestamp time.Time, values []webserver.IndicatorValue)
	Start(port string) error
}
