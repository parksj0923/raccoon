package feed

import (
	"context"
	"errors"
	"fmt"
	"github.com/StudioSol/set"
	"raccoon/interfaces"
	"raccoon/model"
	"raccoon/utils/log"
	"strings"
	"sync"
)

// 몇 가지 에러 상수
var (
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrInsufficientFunds = errors.New("insufficient funds or locked")
	ErrInvalidAsset      = errors.New("invalid asset")
)

type DataFeed struct {
	Data chan model.Candle
	Err  chan error
}

type DataFeedConsumer func(model.Candle)

type DataSubscription struct {
	onCandleClose bool // 봉이 완성된 경우에만 콜백을 하겠다는지 여부
	consumer      DataFeedConsumer
}

type DataFeedSubscription struct {
	exchange               interfaces.Exchange
	Feeds                  *set.LinkedHashSetString      // (pair--timeframe) 세트
	DataFeeds              map[string]*DataFeed          // key=(pair--timeframe), value=channel pair
	SubscriptionsByFeedKey map[string][]DataSubscription // key=(pair--timeframe), value=subscriber list

	ctx    context.Context
	cancel context.CancelFunc
}

// 전체적인 흐름 : New -> Subscribe -> Preload -> Start(Connect)

func NewDataFeed(exchange interfaces.Exchange) *DataFeedSubscription {
	ctx, cancel := context.WithCancel(context.Background())
	return &DataFeedSubscription{
		exchange:               exchange,
		Feeds:                  set.NewLinkedHashSetString(),
		DataFeeds:              make(map[string]*DataFeed),
		SubscriptionsByFeedKey: make(map[string][]DataSubscription),
		ctx:                    ctx,
		cancel:                 cancel,
	}
}

// Subscribe : 구독 등록 (pair, period, consumer callback, onCandleClose)
func (d *DataFeedSubscription) Subscribe(
	pair, period string,
	consumer DataFeedConsumer,
	onCandleClose bool,
) {
	key := d.makeFeedKey(pair, period)

	d.Feeds.Add(key)

	d.SubscriptionsByFeedKey[key] = append(d.SubscriptionsByFeedKey[key], DataSubscription{
		onCandleClose: onCandleClose,
		consumer:      consumer,
	})
}

// Preload : 미리 읽어온 캔들을 구독자에게 전달(옵션)
func (d *DataFeedSubscription) Preload(pair, period string, candles []model.Candle) {
	log.Infof("[SETUP] preloading %d candles for %s-%s", len(candles), pair, period)
	key := d.makeFeedKey(pair, period)

	for i := range candles {
		candles[i].Complete = true
	}

	for _, candle := range candles {
		for _, subscription := range d.SubscriptionsByFeedKey[key] {
			// onCandleClose=true 라면, Complete=true 인 봉만 전달
			if subscription.onCandleClose && !candle.Complete {
				continue
			}
			subscription.consumer(candle)
		}
	}
}

// Start : 고루틴을 띄워 candle/error 수신, 구독자에 전달
func (d *DataFeedSubscription) Start(loadSync bool) {
	// 1) Connect 호출
	d.Connect()

	wg := new(sync.WaitGroup)

	// 2) 모든 feed(key)에 대해 고루틴
	for key, feed := range d.DataFeeds {
		wg.Add(1)

		go func(key string, feed *DataFeed) {
			defer wg.Done()

			for {
				select {
				case <-d.ctx.Done():
					// 취소 신호를 받으면 종료
					return
				case candle, ok := <-feed.Data:
					if !ok {
						// channel 닫힘 => 종료
						return
					}
					// candle 들어옴 => 구독자들에게 브로드캐스트
					for _, subscription := range d.SubscriptionsByFeedKey[key] {
						if subscription.onCandleClose && !candle.Complete {
							continue
						}
						subscription.consumer(candle)
					}

				case err := <-feed.Err:
					if err != nil {
						log.Error("dataFeedSubscription/start: ", err)
						//TODO 에러 상황 => 계속 진행하거나, 필요 시 종료
						for _, subscription := range d.SubscriptionsByFeedKey[key] {
							subscription.consumer(model.Candle{}) // 빈 캔들로 에러 표시 (혹은 별도의 에러 전달 방식)
						}
					}
				}
			}
		}(key, feed)
	}

	log.Infof("Data feed connected.")

	if loadSync {
		// loadSync==true면, wg.Wait() => 모든 feeder가 종료될 때까지 블록
		wg.Wait()
	}
}

// Connect : 실제 CandlesSubscription를 호출하여, (chan Candle, chan error)를 구성
func (d *DataFeedSubscription) Connect() {
	log.Infof("Connecting to the exchange. (Upbit data feed)")
	for feed := range d.Feeds.Iter() {
		pair, period := d.getPairPeriodFromKey(feed)

		cCandle, cErr := d.exchange.CandlesSubscription(pair, period)

		// 3) DataFeed 구성
		d.DataFeeds[feed] = &DataFeed{
			Data: cCandle,
			Err:  cErr,
		}
	}
}

// Stop : 모든 고루틴(구독) 종료
func (d *DataFeedSubscription) Stop() {
	d.cancel() // 모든 고루틴에게 취소 신호 전송
}

// feedKey : (pair, period) => "pair_period"
func (d *DataFeedSubscription) makeFeedKey(pair, period string) string {
	return fmt.Sprintf("%s_%s", pair, period)
}

// pairPeriodFromKey : "pair--period" => (pair, period)
func (d *DataFeedSubscription) getPairPeriodFromKey(key string) (string, string) {
	parts := strings.Split(key, "_")
	return parts[0], parts[1]
}
