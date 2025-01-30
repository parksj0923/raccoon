package test

import (
	"testing"

	"raccoon/consumer"
	"raccoon/model"

	"raccoon/mocks" // 방금 만든 mock_exchange.go가 들어있는 package
)

// TestOrderFeedConsumer_BuyMarketPrice
//   - "시장가 매수" (Upbit: side=buy, type=price, quantity=KRW금액)를 넣었을 때
//     mockExchange가 CreateOrderMarket(...)을 정말 호출하는지 검증
func TestOrderFeedConsumer_BuyMarketPrice(t *testing.T) {
	// 1) Mock Broker 세팅
	mockEx := &mocks.MockExchange{
		// 초기 잔고 10만원
		MockKrw: 100000,
	}

	// 2) OrderFeedConsumer 생성
	ofc := consumer.NewOrderFeedConsumer(mockEx)

	// 3) "시장가 매수" 주문 객체 (type=OrderTypePrice, Price=KRW금액)
	buyOrder := model.Order{
		Pair:  "KRW-DOGE",
		Side:  model.SideTypeBuy,
		Type:  model.OrderTypePrice, // 업비트 시장가 매수는 ord_type=price
		Price: 50000,                // 5만원치 매수
	}

	// 4) OnOrder() 호출
	ofc.OnOrder(buyOrder)

	// 5) 결과 확인
	if mockEx.CreateOrderMarketCount != 1 {
		t.Fatalf("want 1 call of CreateOrderMarket, got %d", mockEx.CreateOrderMarketCount)
	}
	// 실제 Mock에 마지막으로 들어간 주문 확인
	if mockEx.LastCreatedOrder.Side != model.SideTypeBuy {
		t.Errorf("wanted Side=Buy, got %v", mockEx.LastCreatedOrder.Side)
	}
	if mockEx.LastCreatedOrder.Quantity != 50000 {
		t.Errorf("wanted Quantity=50000 (KRW금액), got %v", mockEx.LastCreatedOrder.Quantity)
	}

	// 추가로 mock 잔고가 어떻게 변했는지도 확인 가능
	if mockEx.MockKrw != 50000 {
		t.Errorf("매수 후 KRW 잔고가 50000이어야 하는데, got %v", mockEx.MockKrw)
	}
	if mockEx.MockCoin <= 0 {
		t.Errorf("코인 잔고가 늘어나야 하는데, got %v", mockEx.MockCoin)
	}
}

// 만약 MockExchange의 mockKrw, mockCoin를 Getter로 꺼내고 싶다면
// (지금 예시엔 필드가 소문자(m.mockKrw)라 외부 접근 불가)
// 별도 Getter 함수 만들어 쓰면 됩니다.
