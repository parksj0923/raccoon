package test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"raccoon/exchange"
	"testing"
	"time"
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

func Test_CandlesByLimit(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	res, err := upbit.CandlesByLimit(pairs[0], "3m", 10)
	fmt.Println(res)
}

func Test_CandlesByPeriod(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	layout := "2006-01-02 15:04:05"
	KSTloc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Error("KST 로케이션 로드 실패:", KSTloc)
		return
	}

	startStr := "2025-01-12 13:00:00"
	endStr := "2025-01-12 13:45:00"

	start, err := time.ParseInLocation(layout, startStr, KSTloc)
	if err != nil {
		fmt.Println("Start 날짜 파싱 에러:", err)
		return
	}

	end, err := time.ParseInLocation(layout, endStr, KSTloc)
	if err != nil {
		fmt.Println("End 날짜 파싱 에러:", err)
		return
	}

	res, err := upbit.CandlesByPeriod(pairs[0], "1h", start, end)
	if err != nil {
		t.Error(err)
	}
	require.Equal(t, 3, len(res))

	require.Equal(t, start, res[0].Time)
	require.Equal(t, time.Date(2025, time.January, 12, 13, 21, 0, 0, KSTloc), res[2].Time)
}

func Test_WsConnect(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	receivedChannel, errCh := upbit.CandlesSubscription(pairs[0], "1h")
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

func Test_CandlesSubscription(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}
	c1m, e1m := upbit.CandlesSubscription("KRW-DOGE", "1m")
	c3m, e3m := upbit.CandlesSubscription("KRW-DOGE", "3m")

	// consume
	go func() {
		for {
			select {
			case candle, ok := <-c1m:
				if !ok {
					return
				}
				fmt.Println("[1m] candle =>", candle)
			case err, ok := <-e1m:
				if ok {
					fmt.Println("[1m err] =>", err)
				}
			case candle, ok := <-c3m:
				if !ok {
					return
				}
				fmt.Println("[3m] candle =>", candle)
			case err, ok := <-e3m:
				if ok {
					fmt.Println("[3m err] =>", err)
				}
			}
		}
	}()
	time.Sleep(100 * time.Second)
}
