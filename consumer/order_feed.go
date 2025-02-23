// consumer/order_feed_consumer_broker.go
package consumer

import (
	"fmt"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

type OrderExecutedCallback func(order model.Order, err error)

type OrderFeedConsumerBroker struct {
	broker    interfaces.Exchange
	callbacks []OrderExecutedCallback
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
			executedOrder, err = o.broker.CreateOrderMarket(model.SideTypeBuy, order.Pair, order.Price)
		} else {
			log.Warnf("[OrderFeedConsumerBroker] Unsupported buy order type: %v", order.Type)
			err = fmt.Errorf("unsupported buy order type: %v", order.Type)
		}
	case model.SideTypeSell:
		if order.Type == model.OrderTypeMarket {
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
