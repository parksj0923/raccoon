package test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"raccoon/exchange"
	"raccoon/feed"
	"raccoon/model"
	"testing"
	"time"
)

func Test_Subscribe(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}

	dataFeed := feed.NewDataFeed(upbit)

	dataFeed.Subscribe("KRW-BTC", "1m", func(c model.Candle) {
		fmt.Printf("[1m Candle] %v, high : %v, low : %v, vol : %v\n", c.Time, c.High, c.Low, c.Volume)
	}, true)

	dataFeed.Subscribe("KRW-BTC", "5m", func(c model.Candle) {
		fmt.Printf("[5m Candle] %v, high : %v, low : %v, vol : %v\n", c.Time, c.High, c.Low, c.Volume)
	}, true)

	expected1mKey := "KRW-BTC_1m"
	expected5mKey := "KRW-BTC_5m"

	_, ok := dataFeed.SubscriptionsByFeedKey[expected1mKey]
	require.Equal(t, true, ok)

	_, ok = dataFeed.SubscriptionsByFeedKey[expected5mKey]
	require.Equal(t, true, ok)

}

func Test_PreloadDataFeed(t *testing.T) {

}

func Test_StartAndStop(t *testing.T) {
	upbit, err := exchange.NewUpbit(apiKey, secretKey, pairs)
	if err != nil {
		t.Error(err)
	}

	dataFeed := feed.NewDataFeed(upbit)

	dataFeed.Subscribe("KRW-BTC", "1s", func(c model.Candle) {
		fmt.Printf("[1s Candle] %v, high : %v, low : %v, vol : %v\n", c.Time, c.High, c.Low, c.Volume)
	}, true)

	dataFeed.Subscribe("KRW-BTC", "1m", func(c model.Candle) {
		fmt.Printf("[1m Candle] %v, high : %v, low : %v, vol : %v\n", c.Time, c.High, c.Low, c.Volume)
	}, true)

	dataFeed.Subscribe("KRW-BTC", "2m", func(c model.Candle) {
		fmt.Printf("[5m Candle] %v, high : %v, low : %v, vol : %v\n", c.Time, c.High, c.Low, c.Volume)
	}, true)

	dataFeed.Start(false)

	time.Sleep(130 * time.Second)

	dataFeed.Stop()

}
