package consumer

import (
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
)

// OrderFeedConsumer : OrderFeedSubscription에서 수신된 Order를 처리
type OrderFeedConsumer struct {
	broker interfaces.Exchange // 실제 Broker
}

// NewOrderFeedConsumer : 생성자
func NewOrderFeedConsumer(exchange interfaces.Exchange) *OrderFeedConsumer {
	return &OrderFeedConsumer{
		broker: exchange,
	}
}

func (o *OrderFeedConsumer) OnOrder(order model.Order) {
	log.Infof("[OrderFeedConsumer] Received order: %#v", order)

	// 실제 Broker 주문 실행
	var err error
	switch order.Side {
	case model.SideTypeBuy:
		if order.Type == model.OrderTypePrice {
			// 시장가 매수 (Upbit: Price=KRW금액)
			_, err = o.broker.CreateOrderMarket(model.SideTypeBuy, order.Pair, order.Price)
		} else {
			log.Warnf("[OrderFeedConsumer] Unsupported buy order type: %v", order.Type)
		}
	case model.SideTypeSell:
		if order.Type == model.OrderTypeMarket {
			// 시장가 매도 (Upbit: Volume=order.Quantity)
			_, err = o.broker.CreateOrderMarket(model.SideTypeSell, order.Pair, order.Quantity)
		} else {
			log.Warnf("[OrderFeedConsumer] Unsupported sell order type: %v", order.Type)
		}
	}

	if err != nil {
		log.Errorf("[OrderFeedConsumer] Failed to create order: %v", err)
		// TODO: 알림/DB저장 등
	} else {
		log.Infof("[RaccoonOrderFeedConsumer] Order placed successfully: side=%s, pair=%s", order.Side, order.Pair)
		// TODO: 알림/DB저장 등
	}
}
