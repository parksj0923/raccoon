package test

import (
	"fmt"
	"os"
	"raccoon/model"
	"raccoon/notification"
	"testing"
)

func Test_TelegramNotification(t *testing.T) {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if botToken == "" || chatID == "" {
		fmt.Println("환경 변수 TELEGRAM_BOT_TOKEN과 TELEGRAM_CHAT_ID를 설정하세요.")
		return
	}

	tn := notification.NewTelegramNotifier(botToken, chatID)

	// [성공 케이스] 예제 주문 객체 생성
	successOrder := model.Order{
		Pair:     "KRW-XRP",
		Side:     model.SideTypeBuy, // 매수 주문
		Price:    10000.0,
		Quantity: 50.0,
		// 나머지 필드는 필요에 따라 채워주세요.
	}

	fmt.Println("주문 실행 성공 알림 전송 테스트 중...")
	// 주문 실행 성공 시에는 두번째 인자로 nil 전달
	tn.OrderNotifier(successOrder, nil)

	// [실패 케이스] 예제 주문 객체 생성
	failureOrder := model.Order{
		Pair:     "KRW-XRP",
		Side:     model.SideTypeSell, // 매도 주문
		Price:    11000.0,
		Quantity: 25.0,
		// 나머지 필드는 필요에 따라 채워주세요.
	}

	fmt.Println("주문 실행 실패 알림 전송 테스트 중...")
	// 실패 시에는 에러를 전달 (예시로 fmt.Errorf 사용)
	tn.OrderNotifier(failureOrder, fmt.Errorf("주문 실행 오류: 예시 오류 메시지"))
}
