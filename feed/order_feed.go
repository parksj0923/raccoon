package feed

import (
	"context"
	"fmt"
	"raccoon/model"
	"sync"
)

type OrderFeed struct {
	Data chan model.Order
	Err  chan error
}

type OrderFeedConsumer func(order model.Order)

type OrderSubscription struct {
	consumer    OrderFeedConsumer
	lastOrderId string
}

type OrderFeedSubscription struct {
	OrderFeeds             map[string]*OrderFeed
	SubscriptionsByFeedKey map[string][]OrderSubscription

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex
}

func NewOrderFeed() *OrderFeedSubscription {
	ctx, cancel := context.WithCancel(context.Background())
	return &OrderFeedSubscription{
		OrderFeeds:             make(map[string]*OrderFeed),
		SubscriptionsByFeedKey: make(map[string][]OrderSubscription),
		ctx:                    ctx,
		cancel:                 cancel,
	}
}

// 전체적인 흐름 : New -> Subscribe -> Start -> Publish

func (d *OrderFeedSubscription) Subscribe(pair string, consumer OrderFeedConsumer) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.OrderFeeds[pair]; !ok {
		d.OrderFeeds[pair] = &OrderFeed{
			Data: make(chan model.Order, 100), //버퍼링된 채널로 퍼블리시 블로킹 방지
			Err:  make(chan error, 10),
		}
	}

	if _, ok := d.SubscriptionsByFeedKey[pair]; !ok {
		d.SubscriptionsByFeedKey[pair] = make([]OrderSubscription, 0)
	}
	d.SubscriptionsByFeedKey[pair] = append(d.SubscriptionsByFeedKey[pair], OrderSubscription{
		consumer: consumer,
	})

}

func (d *OrderFeedSubscription) Publish(order model.Order) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if feed, ok := d.OrderFeeds[order.Pair]; ok {
		select {
		case feed.Data <- order:
			return
		case <-d.ctx.Done():
			return
		}

	}
}

func (d *OrderFeedSubscription) Start() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for pair := range d.OrderFeeds {
		go func(pair string, feed *OrderFeed) {
			for {
				select {
				case <-d.ctx.Done():
					close(feed.Data)
					close(feed.Err)
					return
				case order, ok := <-feed.Data:
					if !ok {
						return
					}
					d.deliverToSubscribers(pair, order)
				case err, ok := <-feed.Err:
					if ok {
						//TODO Error handling
						fmt.Println(err)
						return
					}
				}
			}
		}(pair, d.OrderFeeds[pair])
	}
}

func (d *OrderFeedSubscription) deliverToSubscribers(pair string, order model.Order) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	subscriptions, ok := d.SubscriptionsByFeedKey[pair]
	if !ok {
		return
	}

	for _, sub := range subscriptions {
		if sub.lastOrderId == order.ExchangeID {
			continue
		}
		sub.lastOrderId = order.ExchangeID
		//TODO 비동기 처리 필요, but 고루틴으로 하면 order의 순서를 보장 못함. 메세지큐 도입해야함
		sub.consumer(order)
	}
}

func (d *OrderFeedSubscription) Stop() {
	d.cancel()
}
