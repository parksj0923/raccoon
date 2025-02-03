// consumer/order_feed_consumer_broker.go
package consumer

import (
	"fmt"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

type OrderExecutedCallback func(order model.Order, err error)

// OrderFeedConsumerBroker 는 OrderFeedSubscription에서 전달받은 주문 요청을 실제 Broker로 전달하고,
// 주문이 성공적으로 실행(체결)된 후 등록된 콜백들을 호출합니다.
type OrderFeedConsumerBroker struct {
	broker    interfaces.Exchange     // 실제 Broker
	callbacks []OrderExecutedCallback // 주문 실행 완료 후 호출할 콜백 함수들
}

func NewOrderFeedConsumerBroker(exchange interfaces.Exchange) *OrderFeedConsumerBroker {
	return &OrderFeedConsumerBroker{
		broker:    exchange,
		callbacks: make([]OrderExecutedCallback, 0),
	}
}

func (o *OrderFeedConsumerBroker) AddOrderExecutedCallback(cb OrderExecutedCallback) {
	o.callbacks = append(o.callbacks, cb)
}

func (o *OrderFeedConsumerBroker) OnOrder(order model.Order) {
	log.Infof("[OrderFeedConsumerBroker] Received order - Pair: %s, Side: %s, Type: %s, Quantity: %.2f",
		order.Pair, order.Side, order.Type, order.Quantity)

	var executedOrder model.Order
	var err error

	switch order.Side {
	case model.SideTypeBuy:
		if order.Type == model.OrderTypePrice {
			// 시장가 매수 (Upbit의 경우 Price 파라미터가 주문 금액)
			executedOrder, err = o.broker.CreateOrderMarket(model.SideTypeBuy, order.Pair, order.Price)
		} else {
			log.Warnf("[OrderFeedConsumerBroker] Unsupported buy order type: %v", order.Type)
			err = fmt.Errorf("unsupported buy order type: %v", order.Type)
		}
	case model.SideTypeSell:
		if order.Type == model.OrderTypeMarket {
			// 시장가 매도 (Upbit의 경우 Quantity 파라미터가 주문 수량)
			executedOrder, err = o.broker.CreateOrderMarket(model.SideTypeSell, order.Pair, order.Quantity)
		} else {
			log.Warnf("[OrderFeedConsumerBroker] Unsupported sell order type: %v", order.Type)
			err = fmt.Errorf("unsupported sell order type: %v", order.Type)
		}
	}

	for _, cb := range o.callbacks {
		if err != nil {
			cb(order, err)
		} else {
			cb(executedOrder, nil)
		}
	}
}
