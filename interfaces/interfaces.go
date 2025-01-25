package interfaces

import (
	"raccoon/model"
	"time"
)

type Exchange interface {
	Broker
	DataFeeder
}

type Broker interface {
	Account() (model.Asset, error)
	Position(pair string) (asset, quote float64, err error)
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
}

type Notifier interface{}
