package mocks

import (
	"errors"
	"sync"
	"time"

	"raccoon/model"
)

type MockExchange struct {
	mu              sync.Mutex
	MockKrw         float64
	MockCoin        float64
	MockAvgBuyPrice float64

	// 테스트 관찰용
	CreateOrderMarketCount int
	LastCreatedOrder       model.Order
}

func (m *MockExchange) Start() {
	m.CandlesSubscription("KRW-DOGE", "1m")
}
func (m *MockExchange) Stop() {}

func (m *MockExchange) Account() (model.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return model.Asset{
		Balances: []model.Balance{
			{
				Currency:     "KRW",
				Balance:      m.MockKrw,
				Locked:       0,
				AvgBuyPrice:  0,
				UnitCurrency: "KRW",
			},
			{
				Currency:     "DOGE",
				Balance:      m.MockCoin,
				Locked:       0,
				AvgBuyPrice:  m.MockAvgBuyPrice,
				UnitCurrency: "KRW",
			},
		},
	}, nil
}

func (m *MockExchange) OrderChance(pair string) (*model.OrderChance, error) {
	panic("implement me")
}

func (m *MockExchange) Position(pair string) (base, quote, avgBuyPrice float64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MockCoin, m.MockKrw, m.MockAvgBuyPrice, nil
}

func (m *MockExchange) CandlesByLimit(pair, period string, limit int) ([]model.Candle, error) {
	return nil, errors.New("not implemented in mock")
}
func (m *MockExchange) CandlesByPeriod(pair, period string, start, end time.Time) ([]model.Candle, error) {
	return nil, errors.New("not implemented in mock")
}
func (m *MockExchange) CandlesSubscription(pair, period string) (chan model.Candle, chan error) {
	cCandle := make(chan model.Candle)
	cErr := make(chan error)

	return cCandle, cErr
}

func (m *MockExchange) LastQuote(pair string) (float64, error) {
	return 100.0, nil
}

func (m *MockExchange) CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateOrderMarketCount++

	newOrder := model.Order{
		Side:     side,
		Pair:     pair,
		Quantity: quantity,
		Price:    0,
		Type:     model.OrderTypeMarket,
	}
	m.LastCreatedOrder = newOrder

	if side == model.SideTypeBuy {
		if quantity > m.MockKrw {
			return newOrder, errors.New("not enough KRW in mock")
		}
		coinAmount := quantity / 100.0
		m.MockCoin += coinAmount
		m.MockKrw -= quantity
		if m.MockCoin > 0 {
			m.MockAvgBuyPrice = 100.0
		}
	} else {
		if quantity > m.MockCoin {
			return newOrder, errors.New("not enough coin in mock")
		}
		m.MockCoin -= quantity
		krwGained := quantity * 100
		m.MockKrw += krwGained
	}

	return newOrder, nil
}

func (m *MockExchange) CreateOrderLimit(side model.SideType, pair string, quantity, limit float64, tif ...model.TimeInForceType) (model.Order, error) {
	return model.Order{}, errors.New("not implemented in mock")
}
func (m *MockExchange) CreateOrderBest(side model.SideType, pair string, quantity float64, tif ...model.TimeInForceType) (model.Order, error) {
	return model.Order{}, errors.New("not implemented in mock")
}

func (m *MockExchange) Order(pair string, uuidOrIdentifier string, isIdentifier bool) (model.Order, error) {
	return model.Order{}, errors.New("not implemented in mock")
}
func (m *MockExchange) OpenOrders(pair string, limit int) ([]model.Order, error) {
	return nil, nil
}
func (m *MockExchange) ClosedOrders(pair string, limit int) ([]model.Order, error) {
	return nil, nil
}
func (m *MockExchange) Cancel(order model.Order, isIdentifier bool) error {
	return nil
}
func (m *MockExchange) AssetsInfo(pair string) model.AssetInfo {
	return model.AssetInfo{}
}
