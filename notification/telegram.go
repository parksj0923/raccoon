package notification

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"raccoon/model"
)

type TelegramNotifier struct {
	BotToken string
	ChatID   string
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		BotToken: botToken,
		ChatID:   chatID,
	}
}

func (t *TelegramNotifier) SendNotification(message string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	data := url.Values{}
	data.Set("chat_id", t.ChatID)
	data.Set("text", message)

	resp, err := http.PostForm(apiURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	return err
}

// OrderNotifier 는 주문 실행 결과(성공/실패)를 인자로 받아 알림 메시지를 전송합니다.
func (t *TelegramNotifier) OrderNotifier(order model.Order, err error) {
	if err != nil {
		message := fmt.Sprintf("주문 실행 실패:\n종목: %s\n오류: %v", order.Pair, err)
		if sendErr := t.SendNotification(message); sendErr != nil {
			log.Printf("텔레그램 알림 전송 실패: %v\n", sendErr)
		}
	} else {
		var action string
		switch order.Side {
		case model.SideTypeBuy:
			action = "매수"
		case model.SideTypeSell:
			action = "매도"
		default:
			action = "주문"
		}
		message := fmt.Sprintf("주문 체결 성공:\n종목: %s\n동작: %s\n가격: %.2f\n수량: %.8f",
			order.Pair, action, order.Price, order.Quantity)
		if sendErr := t.SendNotification(message); sendErr != nil {
			log.Printf("텔레그램 알림 전송 실패: %v\n", sendErr)
		}
	}
}
