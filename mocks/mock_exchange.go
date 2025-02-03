package mocks

import (
	"errors"
	"sync"
	"time"

	"raccoon/model"
)

// MockExchange 는 interfaces.Exchange(Upbit 등)를 가짜로 흉내내는 구조체입니다.
// - 계좌 잔고(mockKrw, mockCoin) 필드를 둬서 테스트 중에 임의로 설정 가능
// - 주문(CreateOrderMarket 등) 호출 시 호출 횟수/마지막 주문 등을 기록
type MockExchange struct {
	mu              sync.Mutex
	MockKrw         float64
	MockCoin        float64
	MockAvgBuyPrice float64

	// 테스트 관찰용
	CreateOrderMarketCount int
	LastCreatedOrder       model.Order
}

// ----- 간단히 interfaces.Exchange 인터페이스에 필요한 메서드를 구현 -----
// (여기서는 전략·소비자 테스트에 필요한 몇 가지만 대충 짜둠)

// Start / Stop: 실제로 아무것도 안 하되, 꼭 빈 메서드로라도 구현 필요
func (m *MockExchange) Start() {
	m.CandlesSubscription("KRW-DOGE", "1m")
}
func (m *MockExchange) Stop() {}

// Account: 현재 mockKrw만큼의 KRW 잔고가 있다고 가정
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

// Position: (pair="KRW-DOGE" 가정) => (코인 수량, KRW 잔고, 코인 평균매수가) 리턴
func (m *MockExchange) Position(pair string) (base, quote, avgBuyPrice float64, err error) {
	// 간단히 "KRW-DOGE"만 처리
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MockCoin, m.MockKrw, m.MockAvgBuyPrice, nil
}

// CandlesByLimit / CandlesByPeriod / CandlesSubscription 등은
// 여기서는 전략 테스트에 직접 안 쓰면 더미로만 놔둬도 충분합니다.
func (m *MockExchange) CandlesByLimit(pair, period string, limit int) ([]model.Candle, error) {
	return nil, errors.New("not implemented in mock")
}
func (m *MockExchange) CandlesByPeriod(pair, period string, start, end time.Time) ([]model.Candle, error) {
	return nil, errors.New("not implemented in mock")
}
func (m *MockExchange) CandlesSubscription(pair, period string) (chan model.Candle, chan error) {
	cCandle := make(chan model.Candle)
	cErr := make(chan error)

	// 만약 테스트에서 "수동으로 candle을 넣는" 방식을 쓰면,
	// 여기서 따로 goroutine을 안 만들어도 됩니다.
	// 다만, DataFeedSubscription.Start() 쪽에서 select { case <- cCandle: } 로 소비하게 됨.
	// mock에서 WebSocket 흐름을 흉내내고 싶다면, cCandle에 주기적으로 보내는 goroutine을 둘 수도 있음.

	return cCandle, cErr
}

// LastQuote도 더미
func (m *MockExchange) LastQuote(pair string) (float64, error) {
	return 100.0, nil
}

// CreateOrderMarket: 시장가 주문이 호출되면 → 호출 횟수 증가 & 마지막 주문 기록
//   - 업비트 시장가 매수 => side=buy, quantity=KRW금액
//   - 업비트 시장가 매도 => side=sell, quantity=코인수량
func (m *MockExchange) CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateOrderMarketCount++

	// 실제 체결까지 흉내 내고 싶으면, 가령 매수면 mockCoin을 증가시키고 KRW 감소 등 로직도 가능
	// 여기서는 단순히 "주문이 호출되었다"만 기록
	newOrder := model.Order{
		Side:     side,
		Pair:     pair,
		Quantity: quantity,
		Price:    0, // 시장가는 price=0, quantity=금액(매수) or 수량(매도)
		Type:     model.OrderTypeMarket,
	}
	m.LastCreatedOrder = newOrder

	// 매수일 때는 KRW -> 코인 변환 시뮬레이션 가능
	// 여기서는 예시로 1 DOGE = 100 KRW 라고 가정?
	if side == model.SideTypeBuy {
		if quantity > m.MockKrw {
			return newOrder, errors.New("not enough KRW in mock")
		}
		coinAmount := quantity / 100.0 // 임의환산
		m.MockCoin += coinAmount
		m.MockKrw -= quantity
		if m.MockCoin > 0 {
			m.MockAvgBuyPrice = 100.0
		}
	} else {
		// 매도
		if quantity > m.MockCoin {
			return newOrder, errors.New("not enough coin in mock")
		}
		// 코인 전량 매도 => KRW 증가
		m.MockCoin -= quantity
		krwGained := quantity * 100
		m.MockKrw += krwGained
	}

	return newOrder, nil
}

// CreateOrderLimit / CreateOrderBest 등은 여기서 안 쓰면 생략 가능
func (m *MockExchange) CreateOrderLimit(side model.SideType, pair string, quantity, limit float64, tif ...model.TimeInForceType) (model.Order, error) {
	return model.Order{}, errors.New("not implemented in mock")
}
func (m *MockExchange) CreateOrderBest(side model.SideType, pair string, quantity float64, tif ...model.TimeInForceType) (model.Order, error) {
	return model.Order{}, errors.New("not implemented in mock")
}

// Cancel / OpenOrders / ClosedOrders 등도 미사용이면 생략해도 됨
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
