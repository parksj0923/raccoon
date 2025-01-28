package test

import (
	"raccoon/feed"
	"raccoon/model"
	"testing"
	"time"
)

// TestOrderFeedSubscription_SingleSubscriber 테스트는 단일 구독자가 주문을 올바르게 수신하는지 확인합니다.
func TestOrderFeedSubscription_SingleSubscriber(t *testing.T) {

	ofs := feed.NewOrderFeed()
	receivedOrders := make(chan model.Order, 1)
	consumer := func(order model.Order) {
		receivedOrders <- order
	}

	ofs.Subscribe("KRW-BTC", consumer)

	ofs.Start()
	defer ofs.Stop()

	// 테스트 주문 생성 및 퍼블리시
	testOrder := model.Order{
		ID:         12341324,
		Pair:       "KRW-BTC",
		ExchangeID: "ex1",
		Price:      50000.0,
	}

	ofs.Publish(testOrder)

	// 주문이 소비자에게 전달되었는지 확인
	select {
	case received := <-receivedOrders:
		if received.ID != testOrder.ID {
			t.Errorf("Expected order ID %d, got %d", testOrder.ID, received.ID)
		}
	case <-time.After(1 * time.Second):
		t.Error("Did not receive the order within the expected time")
	}
}

func TestOrderFeedSubscription_MultipleOrder(t *testing.T) {
	ofs := feed.NewOrderFeed()
	receivedOrders := make(chan model.Order, 2)
	consumer := func(order model.Order) {
		receivedOrders <- order
	}

	ofs.Subscribe("KRW-DOGE", consumer)

	ofs.Start()
	defer ofs.Stop()

	// 이전 주문 (should be ignored)
	oldOrder := model.Order{
		ID:         1234,
		Pair:       "KRW-DOGE",
		ExchangeID: "ex100",
		Price:      3000.0,
	}

	// 새로운 주문 (should be received)
	newOrder := model.Order{
		ID:         5678,
		Pair:       "KRW-DOGE",
		ExchangeID: "ex101",
		Price:      3100.0,
	}

	ofs.Publish(oldOrder)
	ofs.Publish(newOrder)

	// 첫번째주문이 소비자에게 전달되었는지 확인
	select {
	case received := <-receivedOrders:
		if received.ID != oldOrder.ID {
			t.Errorf("Expected order ID %d, got %d", oldOrder.ID, received.ID)
		}
	case <-time.After(100 * time.Second):
		t.Error("Did not receive the new order within the expected time")
	}

	// 두번째 주문이 잘 들어왔는지 확인
	select {
	case received := <-receivedOrders:
		if received.ID != newOrder.ID {
			t.Errorf("Expected order ID %d, got %d", newOrder.ID, received.ID)
		}
	case <-time.After(100 * time.Second):
		t.Error("Did not receive the new order within the expected time")
	}
}

// TestOrderFeedSubscription_MultipleSubscribers 테스트는 여러 구독자가 주문을 올바르게 수신하는지 확인합니다.
func TestOrderFeedSubscription_MultipleSubscribers(t *testing.T) {
	ofs := feed.NewOrderFeed()
	receivedOrders1 := make(chan model.Order, 1)
	receivedOrders2 := make(chan model.Order, 1)

	consumer1 := func(order model.Order) {
		receivedOrders1 <- order
	}
	consumer2 := func(order model.Order) {
		receivedOrders2 <- order
	}

	ofs.Subscribe("LTC-USD", consumer1)
	ofs.Subscribe("LTC-USD", consumer2)

	defer ofs.Stop()
	ofs.Start()

	// 테스트 주문 생성 및 퍼블리시
	testOrder := model.Order{
		ID:         1234,
		Pair:       "LTC-USD",
		ExchangeID: "ex200",
		Price:      150.0,
	}

	ofs.Publish(testOrder)

	// 첫 번째 소비자가 주문을 받았는지 확인
	select {
	case received := <-receivedOrders1:
		if received.ID != testOrder.ID {
			t.Errorf("Consumer1: Expected order ID %d, got %d", testOrder.ID, received.ID)
		}
	case <-time.After(5 * time.Second):
		t.Error("Consumer1 did not receive the order within the expected time")
	}

	// 두 번째 소비자가 주문을 받았는지 확인
	select {
	case received := <-receivedOrders2:
		if received.ID != testOrder.ID {
			t.Errorf("Consumer2: Expected order ID %d, got %d", testOrder.ID, received.ID)
		}
	case <-time.After(5 * time.Second):
		t.Error("Consumer2 did not receive the order within the expected time")
	}
}

// TestOrderFeedSubscription_Stop 테스트는 시스템을 종료했을 때 더 이상 주문을 수신하지 않는지 확인합니다.
func TestOrderFeedSubscription_Stop(t *testing.T) {
	ofs := feed.NewOrderFeed()

	// 수신된 주문을 확인하기 위한 채널 생성
	receivedOrders := make(chan model.Order, 1)

	// 소비자 정의
	consumer := func(order model.Order) {
		receivedOrders <- order
	}

	// "XRP-USD" 페어에 구독
	ofs.Subscribe("XRP-USD", consumer)

	defer ofs.Stop()
	ofs.Start()

	// 시스템 종료
	ofs.Stop()

	// 주문 퍼블리시
	testOrder := model.Order{
		ID:         1234,
		Pair:       "XRP-USD",
		ExchangeID: "ex300",
		Price:      1.0,
	}

	ofs.Publish(testOrder)

	// 소비자가 주문을 받지 않아야 함
	select {
	case <-receivedOrders:
		t.Error("Did not expect to receive orders after stopping the subscription system")
	case <-time.After(100 * time.Millisecond):
		// 기대한 대로 주문을 받지 않음
	}
}
