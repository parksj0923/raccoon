package model

import "time"

type SideType string
type OrderType string
type TimeInForceType string
type OrderStatusType string

// side (주문 종류)
const (
	SideTypeBuy  SideType = "bid" // 매수
	SideTypeSell SideType = "ask" // 매도
)

// ord_type (주문 타입)
const (
	OrderTypeLimit  OrderType = "limit"  // 지정가
	OrderTypePrice  OrderType = "price"  // 시장가 매수
	OrderTypeMarket OrderType = "market" // 시장가 매도
	OrderTypeBest   OrderType = "best"   // 최유리 주문
)

// time_in_force (IOC/FOK)
const (
	TimeInForceIOC TimeInForceType = "ioc" // Immediate or Cancel
	TimeInForceFOK TimeInForceType = "fok" // Fill or Kill
)

const (
	OrderStatusTypeCanceled OrderStatusType = "cancel"
	OrderStatusTypeDone     OrderStatusType = "done"
	OrderStatusTypeWait     OrderStatusType = "wait"
)

type Order struct {
	ID         int64           `json:"id"`
	ExchangeID string          `json:"exchange_id"`
	Pair       string          `json:"pair"`
	Side       SideType        `json:"side"`
	Type       OrderType       `json:"type"`
	Status     OrderStatusType `json:"status"`
	Price      float64         `json:"price"`
	Quantity   float64         `json:"quantity"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Internal use (Plot)
	RefPrice    float64 `json:"ref_price"`
	Profit      float64 `json:"profit"`
	ProfitValue float64 `json:"profit_value"`
	Candle      Candle  `json:"-"`
}
