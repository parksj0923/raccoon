package interfaces

import "raccoon/model"

type Exchange interface {
	Broker
	Feeder
}

type Broker interface {
	Account() (model.Asset, error)
	Position(pair string) (asset, quote float64, err error)
	Order(pair string, uuidOrIdentifier string, isIdentifier bool) (model.Order, error)
	CreateOrderLimit(side model.SideType, pair string,
		quantity float64, limit float64, tif ...string) (model.Order, error)
	CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error)
	CreateOrderBest(side model.SideType, pair string, quantity float64, tif ...string) (model.Order, error)
	Cancel(order model.Order, isIdentifier bool) error
}

type Feeder interface{}

type Notifier interface{}
