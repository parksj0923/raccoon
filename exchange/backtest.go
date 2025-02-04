package exchange

import "raccoon/model"

type BacktestBroker struct {
	Pair        string
	KRW         float64
	Coin        float64
	AvgBuyPrice float64
}

func NewBackTestBroker(pair string, initialKRW float64) *BacktestBroker {
	return &BacktestBroker{
		Pair:        pair,
		KRW:         initialKRW,
		Coin:        0.0,
		AvgBuyPrice: 0.0,
	}
}

func (b *BacktestBroker) Position(pair string) (asset, quote, avgBuyPrice float64, err error) {
	return b.Coin, b.KRW, b.AvgBuyPrice, nil
}

func (b *BacktestBroker) Account() (model.Asset, error) {
	return model.Asset{}, nil
}
func (b *BacktestBroker) OrderChance(pair string) (*model.OrderChance, error) {
	return &model.OrderChance{}, nil
}
func (b *BacktestBroker) Order(pair string, uuidOrIdentifier string, isIdentifier bool) (model.Order, error) {
	return model.Order{}, nil
}
func (b *BacktestBroker) OpenOrders(pair string, limit int) ([]model.Order, error) {
	return []model.Order{}, nil
}
func (b *BacktestBroker) CreateOrderLimit(side model.SideType, pair string, quantity, limit float64, tif ...model.TimeInForceType) (model.Order, error) {
	return model.Order{}, nil
}
func (b *BacktestBroker) CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error) {
	return model.Order{}, nil
}
func (b *BacktestBroker) CreateOrderBest(side model.SideType, pair string, quantity float64, tif ...model.TimeInForceType) (model.Order, error) {
	return model.Order{}, nil
}
func (b *BacktestBroker) Cancel(order model.Order, isIdentifier bool) error { return nil }
