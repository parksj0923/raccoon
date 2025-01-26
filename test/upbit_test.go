package test

import (
	"fmt"
	"os"
	"raccoon/exchange"
	"testing"
)

var (
	apiKey    = os.Getenv("UPBIT_ACCESS_KEY")
	secretKey = os.Getenv("UPBIT_SECRET_KEY")
	pairs     = []string{"KRW-BTC"}
)

func Test_NewUpBit(t *testing.T) {
	fmt.Println(apiKey, secretKey, pairs)
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(upbit)
}

func Test_GetAccount(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	account, err := upbit.Account()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(account)
}

func Test_WsConnect(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	receivedChannel, errCh := upbit.CandlesSubscription(pairs[0], "1m")
	for {
		select {
		case err := <-errCh:
			if err != nil {
				t.Error(err)
			}
		case candle := <-receivedChannel:
			fmt.Println(candle)
		}
	}
}
