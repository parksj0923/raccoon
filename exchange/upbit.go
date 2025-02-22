package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"raccoon/utils/collection"
	"raccoon/utils/resty"
	"raccoon/utils/tools"
	"strconv"
	"strings"
	"sync"
	"time"

	"raccoon/model"
	"raccoon/utils/auth"
	"raccoon/utils/log"

	"github.com/gorilla/websocket"
)

const (
	upbitBaseREST  = "https://api.upbit.com"
	upbitBaseWS    = "wss://api.upbit.com/websocket/v1"
	maxWSRetries   = 2
	upbitPrivateWS = "wss://api.upbit.com/websocket/v1/private"
	RandomWsUuid   = "0a9b8f1e-0dd9-4a59-adf8-0b7c10008943"
	Candle1s       = "candle.1s"

	CandlePageLimit = 200
)

var KSTLocation, _ = time.LoadLocation("Asia/Seoul")

type Upbit struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup

	// REST
	apiKey    string
	secretKey string
	resty     resty.RestyClient

	// WebSocket
	wsConn    *websocket.Conn
	wsRunning bool
	wsMtx     sync.Mutex

	assetsInfo map[string]model.AssetInfo

	aggregatorMap map[string]*CandleAggregator
}

type CandleAggregator struct {
	pair     string
	period   string
	duration time.Duration

	buffer     map[time.Time]model.Candle
	currentKey time.Time

	candleCh chan model.Candle
	errCh    chan error
}

func NewUpbit(apiKey, secretKey string, pairs []string) (*Upbit, error) {
	ctx, cancel := context.WithCancel(context.Background())
	restyClient := resty.NewDefaultRestyClient(true, 10*time.Second)
	up := &Upbit{
		ctx:           ctx,
		resty:         restyClient,
		cancelFunc:    cancel,
		apiKey:        apiKey,
		secretKey:     secretKey,
		assetsInfo:    make(map[string]model.AssetInfo),
		aggregatorMap: make(map[string]*CandleAggregator),
	}
	log.Info("[SETUP] Using Upbit exchange with pre-fetched pairs")
	for _, pair := range pairs {
		pair = strings.ToUpper(pair) // "KRW-BTC"
		chance, err := up.OrderChance(pair)
		if err != nil {
			log.Errorf("[UPBIT] Failed to fetch upbit exchange pair %s: %v", pair, err)
			continue
		}
		ai, err := convertChanceToAssetInfo(chance)
		if err != nil {
			log.Errorf("[UPBIT] Failed to convert upbit exchange pair %s: %v", pair, err)
			continue
		}
		up.assetsInfo[pair] = ai
	}
	log.Info("[SETUP] Using Upbit exchange (single-struct)")

	return up, nil
}

func (u *Upbit) Account() (model.Asset, error) {
	body, err := u.requestUpbitGET(u.ctx, "/v1/accounts", nil)
	if err != nil {
		return model.Asset{}, err
	}
	var accResp []model.AccountResponse
	if err := json.Unmarshal(body, &accResp); err != nil {
		return model.Asset{}, fmt.Errorf("account parse: %w", err)
	}

	res := collection.Map(accResp, func(t model.AccountResponse) model.Balance {
		balance, _ := strconv.ParseFloat(t.Balance, 64)
		locked, _ := strconv.ParseFloat(t.Locked, 64)
		avgBuyPrice, _ := strconv.ParseFloat(t.AvgBuyPrice, 64)
		return model.Balance{
			Currency:     t.Currency,
			Balance:      balance,
			Locked:       locked,
			AvgBuyPrice:  avgBuyPrice,
			UnitCurrency: t.UnitCurrency,
		}
	})

	return model.Asset{Balances: res}, nil
}

func (u *Upbit) Position(pair string) (asset, quote, avgBuyPrice float64, err error) {
	acc, err := u.Account()
	if err != nil {
		return 0, 0, 0, err
	}
	base, quoteAsset := SplitAssetQuote(pair)
	var baseBal, quoteBal, avgBuyPriceBal float64
	for _, b := range acc.Balances {
		if strings.EqualFold(b.Currency, base) {
			baseBal = b.Balance + b.Locked
			avgBuyPriceBal = b.AvgBuyPrice

		}
		if strings.EqualFold(b.Currency, quoteAsset) {
			quoteBal = b.Balance + b.Locked

		}
	}
	return baseBal, quoteBal, avgBuyPriceBal, nil
}

func (u *Upbit) OrderChance(pair string) (*model.OrderChance, error) {
	params := map[string]interface{}{
		"market": pair,
	}
	body, err := u.requestUpbitGET(u.ctx, "/v1/orders/chance", params)
	if err != nil {
		return nil, err
	}
	var resp model.OrderChance
	if e := json.Unmarshal(body, &resp); e != nil {
		return nil, e
	}
	if resp.Market.ID == "" {
		return nil, errors.New("invalid chance response")
	}
	return &resp, nil
}

func (u *Upbit) Order(pair string, uuidOrIdentifier string, isIdentifier bool) (model.Order, error) {
	params := map[string]interface{}{}
	if isIdentifier {
		params["identifier"] = uuidOrIdentifier
	} else {
		params["uuid"] = uuidOrIdentifier
	}

	body, err := u.requestUpbitGET(u.ctx, "/v1/order", params)
	if err != nil {
		return model.Order{}, err
	}
	var orderResp model.OrderResponse
	if err := json.Unmarshal(body, &orderResp); err != nil {
		return model.Order{}, err
	}
	return convertOrderToModelOrder(orderResp, pair), nil
}

func (u *Upbit) OpenOrders(pair string, limit int) ([]model.Order, error) {
	params := map[string]interface{}{
		"market":   pair,
		"states[]": "wait",
		"limit":    limit,
		"order_by": "desc",
	}
	body, err := u.requestUpbitGET(u.ctx, "/v1/orders/open", params)
	if err != nil {
		return nil, err
	}
	var resp []model.OrdersResponse
	if e := json.Unmarshal(body, &resp); e != nil {
		return nil, e
	}

	return convertMultiOrdersToModelOrders(resp, pair), nil
}

func (u *Upbit) ClosedOrders(pair string, limit int) ([]model.Order, error) {
	// 미구현
	return nil, errors.New("not implemented: Upbit orders list")
}

func (u *Upbit) CreateOrderLimit(side model.SideType, pair string,
	quantity float64, limit float64, tif ...model.TimeInForceType) (model.Order, error) {

	validPrice := validatePrice(limit, pair)

	params := map[string]interface{}{
		"market":   pair,
		"side":     string(side),
		"ord_type": string(model.OrderTypeLimit),
		"price":    floatToString(validPrice),
		"volume":   floatToString(quantity),
	}
	if len(tif) == 1 {
		params["time_in_force"] = string(tif[0])
	}
	body, err := u.requestUpbitPOST(u.ctx, "/v1/orders", params)
	if err != nil {
		return model.Order{}, err
	}
	var resp model.OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return model.Order{}, err
	}
	return convertOrderToModelOrder(resp, pair), nil
}

func (u *Upbit) CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error) {
	// Upbit 시장가 매수 => ord_type="price", side="bid", quantity = price=금액, volume=""
	// Upbit 시장가 매도 => ord_type="market", side="ask", quantity = volume=수량, price=""
	if side == model.SideTypeBuy {
		feeRate, err := u.getFeeRateForOrder(pair, side, true)
		if err != nil {
			log.Errorf("[Upbit] getFeeRateForOrder failed: %v", err)
			return model.Order{}, fmt.Errorf("수수료율 조회 실패: %w", err)
		}
		effectiveAmount := quantity / (1 + feeRate)
		validPrice := validatePrice(effectiveAmount, pair)
		log.Infof("[Upbit] 매수 주문 전: 금액=%.2f, feeRate=%.5f, 실제 주문금액=%.2f", quantity, feeRate, validPrice)

		params := map[string]interface{}{
			"market":   pair,
			"side":     string(side),
			"ord_type": string(model.OrderTypePrice),
			"price":    floatToString(validPrice),
		}
		body, err := u.requestUpbitPOST(u.ctx, "/v1/orders", params)
		if err != nil {
			return model.Order{}, err
		}
		var resp model.OrderResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return model.Order{}, err
		}
		return convertOrderToModelOrder(resp, pair), nil
	} else {
		params := map[string]interface{}{
			"market":   pair,
			"side":     string(side),
			"ord_type": string(model.OrderTypeMarket),
			"volume":   floatToString(quantity),
		}
		body, err := u.requestUpbitPOST(u.ctx, "/v1/orders", params)
		if err != nil {
			return model.Order{}, err
		}
		var resp model.OrderResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return model.Order{}, err
		}
		return convertOrderToModelOrder(resp, pair), nil
	}
}

func (u *Upbit) CreateOrderBest(side model.SideType, pair string, quantity float64, tif ...model.TimeInForceType) (model.Order, error) {
	// ioc =>
	//		매수 -> tif=ioc, side=bid, ord_type=best, quantity=price
	// 		매도 -> tif=ioc, side=ask, ord_type=best, quantity=volume
	// fork =>
	//		매수 -> tif=fok, side=bid, ord_type=best, quantity=price
	//		매도 -> tif=fok, side=ask, ord_type=best, quantity=volume
	if len(tif) != 1 {
		return model.Order{}, fmt.Errorf("tif must be exist and exactly one parameter")
	}

	params := map[string]interface{}{
		"market":        pair,
		"side":          string(side),
		"ord_type":      string(model.OrderTypeBest),
		"time_in_force": string(tif[0]),
	}
	if side == model.SideTypeBuy {
		params["price"] = floatToString(quantity)
	} else {
		params["volume"] = floatToString(quantity)
	}

	body, err := u.requestUpbitPOST(u.ctx, "/v1/orders", params)
	if err != nil {
		return model.Order{}, err
	}
	var resp model.OrderResponse
	if e := json.Unmarshal(body, &resp); e != nil {
		return model.Order{}, e
	}
	return convertOrderToModelOrder(resp, pair), nil
}

func (u *Upbit) Cancel(order model.Order, isIdentifier bool) error {
	params := map[string]interface{}{}
	if isIdentifier {
		params["identifier"] = order.ExchangeID
	} else {
		params["uuid"] = order.ExchangeID
	}
	_, err := u.requestUpbitDELETE(u.ctx, "/v1/order", params)
	return err
}

func (u *Upbit) AssetsInfo(pair string) model.AssetInfo {

	pair = strings.ToUpper(pair)
	if info, ok := u.assetsInfo[pair]; ok {
		return info
	}
	resp, err := u.OrderChance(pair)
	if err != nil {
		return model.AssetInfo{}
	}
	result, err := convertChanceToAssetInfo(resp)
	if err != nil {
		return model.AssetInfo{}
	}
	return result
}

func (u *Upbit) LastQuote(pair string) (float64, error) {
	// GET /v1/ticker?markets=KRW-BTC
	params := map[string]interface{}{}
	params["market"] = pair
	body, err := u.requestUpbitGET(u.ctx, "/v1/ticker", params)

	if err != nil {
		return 0, err
	}
	var res []model.CurrentTicker
	if err := json.Unmarshal(body, &res); err != nil {
		return 0, err
	}
	if len(res) < 1 {
		return 0, fmt.Errorf("no ticker data for %s", pair)
	}
	return res[0].TradePrice, nil
}

func (u *Upbit) CandlesByLimit(pair, period string, limit int) ([]model.Candle, error) {
	if limit > CandlePageLimit {
		return nil, fmt.Errorf("candles limit exceeds 200")
	}
	// 1) period 파싱 -> Upbit candles endpoint
	endpoint, err := tools.MapPeriodToCandleEndpoint(period)
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{
		"market": pair,
		"count":  limit,
	}
	body, err := u.requestUpbitGET(u.ctx, "/v1/candles/"+endpoint, params)
	if err != nil {
		return nil, err
	}

	var raw []model.QuotationCandle
	if e := json.Unmarshal(body, &raw); e != nil {
		return nil, e
	}
	if len(raw) == 0 {
		return nil, nil
	}

	candles := make([]model.Candle, 0, len(raw))
	for _, r := range raw {
		t, _ := time.ParseInLocation("2006-01-02T15:04:05", r.CandleDateTimeKST, KSTLocation)
		c := model.Candle{
			Pair:      pair,
			Time:      t,
			UpdatedAt: t,
			Open:      r.OpeningPrice,
			High:      r.HighPrice,
			Low:       r.LowPrice,
			Close:     r.TradePrice,
			Volume:    r.CandleAccTradeVolume,
			Complete:  true, // 이미 완료된 봉
			Metadata:  map[string]float64{},
		}
		candles = append(candles, c)
	}
	// sorting ascending
	collection.Sort(candles, func(a, b model.Candle) bool {
		return a.Time.Unix() < b.Time.Unix()
	})
	return candles, nil
}

func (u *Upbit) CandlesByPeriod(pair, period string, start, end time.Time) ([]model.Candle, error) {
	endpoint, err := tools.MapPeriodToCandleEndpoint(period)
	if err != nil {
		return nil, err
	}

	var allCandles []model.Candle
	toTime := end

	for {
		toStr := toTime.Format("2006-01-02T15:04:05") + "+09:00"
		params := map[string]interface{}{
			"market": pair,
			"count":  CandlePageLimit,
			"to":     toStr,
		}

		body, err := u.requestUpbitGET(u.ctx, "/v1/candles/"+endpoint, params)
		if err != nil {
			return nil, err
		}
		var raw []model.QuotationCandle
		if e := json.Unmarshal(body, &raw); e != nil {
			return nil, e
		}
		if len(raw) == 0 {
			break
		}

		for _, r := range raw {
			t, _ := time.ParseInLocation("2006-01-02T15:04:05", r.CandleDateTimeKST, KSTLocation)
			c := model.Candle{
				Pair:      pair,
				Time:      t,
				UpdatedAt: t,
				Open:      r.OpeningPrice,
				High:      r.HighPrice,
				Low:       r.LowPrice,
				Close:     r.TradePrice,
				Volume:    r.CandleAccTradeVolume,
				Complete:  true,
				Metadata:  map[string]float64{},
			}
			allCandles = append(allCandles, c)
		}

		oldest := raw[len(raw)-1]
		oldestTime, _ := time.ParseInLocation("2006-01-02T15:04:05", oldest.CandleDateTimeKST, KSTLocation)

		if !oldestTime.After(start) {
			break
		}
		toTime = oldestTime
		time.Sleep(500 * time.Millisecond)
	}

	collection.Sort(allCandles, func(a, b model.Candle) bool {
		return a.Time.Unix() < b.Time.Unix()
	})

	var result []model.Candle
	for _, c := range allCandles {
		if c.Time.Equal(start) || c.Time.Equal(end) ||
			(c.Time.After(start) && c.Time.Before(end)) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (u *Upbit) CandlesSubscription(pair, period string) (chan model.Candle, chan error) {
	key := pair + "_" + period

	if agg, ok := u.aggregatorMap[key]; ok {
		// 웹소켓도 이미 돌고 있다고 가정
		return agg.candleCh, agg.errCh
	}

	dur, err := tools.ParseTimeframeToDuration(period)
	if err != nil {
		cch := make(chan model.Candle)
		ech := make(chan error, 1)
		ech <- fmt.Errorf("invalid timeframe: %s", period)
		close(ech)
		return cch, ech
	}

	agg := &CandleAggregator{
		pair:     pair,
		period:   period,
		duration: dur,
		buffer:   make(map[time.Time]model.Candle),
		candleCh: make(chan model.Candle),
		errCh:    make(chan error),
	}
	u.aggregatorMap[key] = agg

	now := time.Now().In(KSTLocation)
	agg.initCurrentKey(now)
	diff := agg.currentKey.Sub(now)
	if diff < agg.duration {
		prevStart := agg.currentKey.Add(-agg.duration)
		log.Infof("[CandlesSubscription] check: currentKey=%v, now=%v => Preload[%v~%v)",
			agg.currentKey, now, prevStart, now)

		if prevStart.Before(now) {
			candles, err2 := u.CandlesByPeriod(pair, period, prevStart, now)
			if err2 != nil {
				log.Warnf("Preload fetch error: %v (ignored)", err2)
			} else {
				for _, c := range candles {
					c.Complete = false
					agg.push1sCandle(c)
				}
				log.Infof("[CandlesSubscription] Preload done. total=%d", len(candles))
			}
		}
	} else {
		log.Infof("[CandlesSubscription] skip preload. diff=%v >= duration=%v", diff, agg.duration)
	}

	go u.wsRunIfNeeded()

	return agg.candleCh, agg.errCh
}
func (agg *CandleAggregator) initCurrentKey(t time.Time) {
	t0, _ := tools.TruncateKST(t, agg.duration)
	if !t0.Equal(t) {
		t0 = t0.Add(agg.duration)
	}
	agg.currentKey = t0
}

func (u *Upbit) Start() {
	go u.wsRunIfNeeded()
}

func (u *Upbit) Stop() {
	u.cancelFunc()
	u.wsMtx.Lock()
	if u.wsConn != nil {
		u.wsConn.Close()
	}
	u.wsMtx.Unlock()

	u.wg.Wait()

	for _, agg := range u.aggregatorMap {
		close(agg.candleCh)
		close(agg.errCh)
	}
	log.Info("[Upbit] stopped")
}

func (u *Upbit) wsRunIfNeeded() {
	u.wsMtx.Lock()
	defer u.wsMtx.Unlock()
	if u.wsRunning {
		return
	}
	u.wsRunning = true

	go u.runWebsocket()
}

func (u *Upbit) runWebsocket() {
	defer func() {
		u.wsMtx.Lock()
		u.wsRunning = false
		u.wsMtx.Unlock()
		u.wg.Done()
	}()
	u.wg.Add(1)

connect:
	// 연결
	conn, _, err := websocket.DefaultDialer.Dial(upbitBaseWS, nil)
	if err != nil {
		u.broadcastErr(err)
		log.Errorf("Upbit ws dial fail: %v", err)
		return
	}
	u.wsConn = conn
	log.Info("[UpbitWS] connected")

	conn.SetPongHandler(func(appData string) error {
		// PONG 수신 시점부터 2분 뒤까지는 유효하다고 설정(Upbit가 120초 타임아웃)
		conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		return nil
	})

	keepaliveCtx, keepaliveCancel := context.WithCancel(u.ctx)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer keepaliveCancel() // 이 고루틴이 끝나면 context도 종료

		for {
			select {
			case <-keepaliveCtx.Done():
				return
			case <-ticker.C:
				// write ping frame
				// 세 번째 파라미터는 deadline (시간 제한)
				if err := conn.WriteControl(
					websocket.PingMessage,
					[]byte("ping"),
					time.Now().Add(5*time.Second),
				); err != nil {
					log.Warnf("[UpbitWS] ping error: %v", err)
					return
				}
			}
		}
	}()

	// 초기 read deadline 설정
	conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

	pairsSet := make(map[string]bool)
	for k := range u.aggregatorMap {
		// k = "KRW-BTC_1m" => "KRW-BTC"
		splits := strings.Split(k, "_")
		if len(splits) < 2 {
			continue
		}
		p := splits[0]
		pairsSet[p] = true
	}
	var codes []string
	for p := range pairsSet {
		codes = append(codes, strings.ToUpper(p))
	}

	subMsg := []interface{}{
		map[string]string{"ticket": RandomWsUuid},
		map[string]interface{}{
			"type":  Candle1s,
			"codes": codes,
		},
		map[string]string{"format": "DEFAULT"},
	}
	if e := conn.WriteJSON(subMsg); e != nil {
		u.broadcastErr(e)
		log.Errorf("[UpbitWS] write sub fail: %v", e)
		conn.Close()
		return
	}

	var retries int
	// read loop
	for {
		select {
		case <-u.ctx.Done():
			log.Info("[UpbitWS] context done => close ws")
			conn.Close()
			return
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Errorf("[UpbitWS] read err: %v", err)
				if retries < maxWSRetries {
					retries++
					log.Warnf("[UpbitWS] retrying... attempt=%d", retries)
					time.Sleep(1 * time.Second)
					conn.Close()
					keepaliveCancel()
					goto connect
				} else {
					u.broadcastErr(fmt.Errorf("read fail after %d retries: %w", maxWSRetries, err))
					conn.Close()
					keepaliveCancel()
					return
				}
			}
			conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			u.handleCandle1s(msg)
		}
	}
}

func (u *Upbit) handleCandle1s(msg []byte) {
	var base model.WSCandleBase
	if e := json.Unmarshal(msg, &base); e != nil {
		log.Warnf("ws base parse fail: %v", e)
		return
	}
	if base.Error.Name != "" {
		errMsg := fmt.Errorf("[UpbitWS] candle.1s error: %s - %s", base.Error.Name, base.Error.Message)
		log.Errorf(errMsg.Error())
		u.broadcastErr(errMsg)
		return
	}
	if base.Type != Candle1s {
		return
	}

	var raw model.WSCandle
	if e := json.Unmarshal(msg, &raw); e != nil {
		log.Warnf("candle.1s parse fail: %v", e)
		return
	}

	t, _ := time.ParseInLocation("2006-01-02T15:04:05", raw.CandleDateTimeKst, KSTLocation)
	candle := model.Candle{
		Pair:      raw.Code,
		Time:      t,
		UpdatedAt: t,
		Open:      raw.OpeningPrice,
		High:      raw.HighPrice,
		Low:       raw.LowPrice,
		Close:     raw.TradePrice,
		Volume:    raw.CandleAccTradeVolume,
		Complete:  true,
		Metadata:  map[string]float64{},
	}

	// aggregatorMap => push
	for k, agg := range u.aggregatorMap {
		parts := strings.Split(k, "_")
		if len(parts) < 2 {
			continue
		}
		if strings.EqualFold(parts[0], raw.Code) {
			partial, final, isFinal := agg.push1sCandle(candle)

			if partial.Volume > 0 {
				agg.candleCh <- partial
			}

			if isFinal && final.Volume > 0 {
				agg.candleCh <- final
			}
		}
	}
}

func (agg *CandleAggregator) push1sCandle(c model.Candle) (partial model.Candle, final model.Candle, isFinal bool) {
	// 1) duration=0 => "1s" timeframe
	if agg.duration == 0 {
		agg.buffer[c.Time] = c
		// 1초봉 그대로 완성
		return c, model.Candle{}, true
	}

	// 2) buffer에 저장(override)
	agg.buffer[c.Time] = c

	// 3) 초기에 currentKey가 0이면, "다음 정각"으로 맞춘다
	//    예: c.Time=13:29:12, truncate(1h)=13:00,
	//        다음 경계=14:00 (즉 t0.Add(duration))
	if agg.currentKey.IsZero() {
		agg.initCurrentKey(c.Time)
	}

	// 4) "부분봉"(partialCandle)은 [currentKey - duration, c.Time) 사이 데이터
	//    -> 아직 완성되지 않은 구간
	//    예: currentKey=14:00 => partial은 [13:00, 13:29] ~ c.Time(실제)
	partialStart := agg.currentKey.Add(-agg.duration)
	partial = agg.buildPartialCandle(partialStart, c.Time)
	partial.Complete = false

	// 5) finalCandle: 만약 c.Time >= agg.currentKey면, "정각"에 도달하거나 지났으므로 이전 구간 완성
	isFinal = false
	if !c.Time.Before(agg.currentKey) {
		// (a) 이전 구간: [currentKey - duration, currentKey)
		finalStart := agg.currentKey.Add(-agg.duration)
		final = agg.aggregateBuffer(finalStart)
		final.Complete = true
		isFinal = true

		// (b) buffer에서 이전 구간 데이터 제거
		agg.removeOldSeconds(finalStart)

		// (c) 다음 정각으로 이동
		agg.currentKey = agg.currentKey.Add(agg.duration)
	}

	return partial, final, isFinal
}

// buildPartialCandle : [startKey, now) 구간을 합산, 아직 완료되지 않은 봉(Complete=false)
func (agg *CandleAggregator) buildPartialCandle(startKey, now time.Time) model.Candle {
	// 부분봉을 위한 임시합산
	//   interval = [startKey, now) (단, now가 startKey + duration 보다 크면 사실 final이 될 것)
	var secs []model.Candle
	for t, sc := range agg.buffer {
		if (t.Equal(startKey) || t.After(startKey)) && t.Before(now) {
			secs = append(secs, sc)
		}
	}
	if len(secs) == 0 {
		return model.Candle{
			Pair:  agg.pair,
			Time:  now,
			Close: 0,
		}
	}

	collection.Sort(secs, func(a, b model.Candle) bool {
		return a.Time.Before(b.Time)
	})
	first := secs[0]
	out := model.Candle{
		Pair:      first.Pair,
		Time:      now, // 부분봉의 시각은 '현재' 시각 (혹은 startKey?)
		UpdatedAt: now,
		Open:      first.Open,
		High:      first.High,
		Low:       first.Low,
		Close:     first.Close,
		Volume:    0,
		Complete:  false,
		Metadata:  make(map[string]float64),
	}
	for i, sc := range secs {
		if i > 0 {
			if sc.High > out.High {
				out.High = sc.High
			}
			if sc.Low < out.Low {
				out.Low = sc.Low
			}
		}
		out.Close = sc.Close
		out.Volume += sc.Volume
	}
	return out
}

// removeOldSeconds : remove all second in the old interval
func (agg *CandleAggregator) removeOldSeconds(minKey time.Time) {
	start := minKey
	end := minKey.Add(agg.duration)

	for sec := range agg.buffer {

		if (sec.After(start) || sec.Equal(start)) && sec.Before(end) {
			delete(agg.buffer, sec)
		}
	}
}

// aggregateBuffer : [minKey, minKey+duration) 완성봉
func (agg *CandleAggregator) aggregateBuffer(minKey time.Time) model.Candle {
	start := minKey
	end := minKey.Add(agg.duration)

	var secs []model.Candle
	for _, sc := range agg.buffer {
		if sc.Time.After(start) && sc.Time.Before(end) {
			secs = append(secs, sc)
		}
	}
	if len(secs) == 0 {
		return model.Candle{
			Pair: agg.pair,
			Time: minKey,
		}
	}
	// sort
	collection.Sort(secs, func(a, b model.Candle) bool {
		return a.Time.Before(b.Time)
	})

	first := secs[0]
	out := model.Candle{
		Pair:      first.Pair,
		Time:      minKey,
		UpdatedAt: minKey,
		Open:      first.Open,
		High:      first.High,
		Low:       first.Low,
		Close:     first.Close,
		Volume:    first.Volume,
		Complete:  true,
		Metadata:  make(map[string]float64),
	}
	for i := 1; i < len(secs); i++ {
		sc := secs[i]
		if sc.High > out.High {
			out.High = sc.High
		}
		if sc.Low < out.Low {
			out.Low = sc.Low
		}
		out.Close = sc.Close
		out.Volume += sc.Volume
	}
	return out
}

func (u *Upbit) broadcastErr(err error) {
	for _, agg := range u.aggregatorMap {
		go func(a *CandleAggregator) {
			select {
			case agg.errCh <- err:
			default:
			}
		}(agg)
	}
}

// -----------------------------------------------------------------------------
// 내부 헬퍼
// -----------------------------------------------------------------------------

func SplitAssetQuote(pair string) (base, quote string) {
	// 예: "KRW-BTC" -> ("KRW", "BTC") -> base = "BTC", quote = "KRW"
	parts := strings.Split(pair, "-")
	if len(parts) == 2 {
		return parts[1], parts[0]
	}
	return pair, ""
}

func convertOrderToModelOrder(o model.OrderResponse, pair string) model.Order {
	priceF, _ := strconv.ParseFloat(o.Price, 64)
	volF, _ := strconv.ParseFloat(o.Volume, 64)
	var createdTime time.Time
	if o.CreatedAt != "" {
		// Upbit가 "2024-06-13T10:28:36+09:00" 형태이므로, time.RFC3339 파싱 가능
		t, err := time.ParseInLocation(time.RFC3339, o.CreatedAt, KSTLocation)
		if err == nil {
			createdTime = t
		} else {
			log.Warnf("[Upbit] convert order create time fail: %v", o)
			createdTime = time.Now()
		}
	}
	return model.Order{
		ExchangeID: o.UUID,
		Pair:       pair,
		Side:       model.SideType(o.Side),
		Type:       model.OrderType(o.OrdType),
		Status:     model.OrderStatusType(o.State),
		Price:      priceF,
		Quantity:   volF,
		CreatedAt:  createdTime,
		UpdatedAt:  time.Now(),
	}
}

func convertMultiOrdersToModelOrders(orders []model.OrdersResponse, pair string) []model.Order {
	results := collection.Map(orders, func(o model.OrdersResponse) model.Order {
		priceF, _ := strconv.ParseFloat(o.Price, 64)
		volF, _ := strconv.ParseFloat(o.Volume, 64)
		var createdTime time.Time
		if o.CreatedAt != "" {
			// Upbit가 "2024-06-13T10:28:36+09:00" 형태이므로, time.RFC3339 파싱 가능
			t, err := time.ParseInLocation(time.RFC3339, o.CreatedAt, KSTLocation)
			if err == nil {
				createdTime = t
			} else {
				log.Warnf("[Upbit] convert order create time fail: %v", o)
				createdTime = time.Now()
			}
		}
		return model.Order{
			ExchangeID: o.UUID,
			Pair:       pair,
			Side:       model.SideType(o.Side),
			Type:       model.OrderType(o.OrdType),
			Status:     model.OrderStatusType(o.State),
			Price:      priceF,
			Quantity:   volF,
			CreatedAt:  createdTime,
			UpdatedAt:  time.Now(),
		}
	})
	return results
}

// requestUpbitGET : Upbit JWT + GET
func (u *Upbit) requestUpbitGET(ctx context.Context, path string, params map[string]interface{}) ([]byte, error) {
	full := upbitBaseREST + path
	token, err := auth.GenerateJWT(u.apiKey, u.secretKey, params)
	if err != nil {
		return nil, err
	}
	header := map[string]string{
		"Authorization": "Bearer " + token,
	}

	var qParams []resty.QueryParam
	for k, v := range params {
		qParams = append(qParams, resty.QueryParam{Key: k, Value: v})
	}

	resp, err := u.resty.
		MakeRequest(ctx, nil, header).
		Get(full, qParams...)

	if err != nil {
		return nil, fmt.Errorf("API 호출 실패: %w", err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// requestUpbitPOST : Upbit JWT + POST
func (u *Upbit) requestUpbitPOST(ctx context.Context, path string, params map[string]interface{}) ([]byte, error) {
	full := upbitBaseREST + path
	token, err := auth.GenerateJWT(u.apiKey, u.secretKey, params)
	if err != nil {
		return nil, err
	}
	header := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	resp, err := u.resty.
		MakeRequest(ctx, params, header).
		Post(full)

	if err != nil {
		return nil, fmt.Errorf("API 호출 실패: %w", err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// requestUpbitDELETE
func (u *Upbit) requestUpbitDELETE(ctx context.Context, path string, params map[string]interface{}) ([]byte, error) {
	full := upbitBaseREST + path

	token, err := auth.GenerateJWT(u.apiKey, u.secretKey, params)
	if err != nil {
		return nil, err
	}
	header := map[string]string{
		"Authorization": "Bearer " + token,
	}
	var qParams []resty.QueryParam
	for k, v := range params {
		qParams = append(qParams, resty.QueryParam{Key: k, Value: v})
	}

	resp, err := u.resty.
		MakeRequest(ctx, nil, header).
		Delete(full, qParams...)

	if err != nil {
		return nil, fmt.Errorf("API 호출 실패: %w", err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// floatToString
func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func convertChanceToAssetInfo(ch *model.OrderChance) (model.AssetInfo, error) {
	// 예: MinTotal = "5000" (KRW)
	minQuote, _ := strconv.ParseFloat(ch.Market.Bid.MinTotal, 64)
	maxQuote, _ := strconv.ParseFloat(ch.Market.MaxTotal, 64)

	// base="BTC", quote="KRW" 같은 식
	base := ch.Market.Ask.Currency
	quote := ch.Market.Bid.Currency
	var stepSize, tickSize float64
	if quote == "KRW" {
		tickSize = 0.1
	} else {
		tickSize = 0.1
	}
	if base == "BTC" || base == "ETH" {
		stepSize = 0.00000001
	} else {
		stepSize = 0.001
	}

	return model.AssetInfo{
		BaseAsset:          base,
		QuoteAsset:         quote,
		MinPrice:           minQuote,
		MaxPrice:           maxQuote,
		StepSize:           stepSize,
		TickSize:           tickSize,
		QuotePrecision:     2,
		BaseAssetPrecision: 8,
	}, nil
}

// validatePrice는 새 호가 정책(2024-10-14 기준)에 따라 주문 가격을 내림(floor) 처리합니다.
// 가격 구간별 주문 단위는 아래와 같습니다:
//
// 2,000,000 이상                        : 1,000
// 1,000,000 이상 ~ 2,000,000 미만       : 500
// 500,000 이상 ~ 1,000,000 미만         : 100
// 100,000 이상 ~ 500,000 미만           : 50
// 10,000 이상 ~ 100,000 미만            : 10
// 1,000 이상 ~ 10,000 미만              : 1
// 100 이상 ~ 1,000 미만 (일반)           : 0.1
// 100 이상 ~ 1,000 미만 (특별종목)         : 1
// 10 이상 ~ 100 미만                    : 0.01
// 1 이상 ~ 10 미만                      : 0.001
// 0.1 이상 ~ 1 미만                     : 0.0001
// 0.01 이상 ~ 0.1 미만                  : 0.00001
// 0.001 이상 ~ 0.01 미만                : 0.000001
// 0.0001 이상 ~ 0.001 미만              : 0.0000001
// 0.0001 미만                          : 0.00000001
func validatePrice(price float64, pair string) float64 {
	var unit float64
	switch {
	case price >= 2000000:
		unit = 1000
	case price >= 1000000:
		unit = 500
	case price >= 500000:
		unit = 100
	case price >= 100000:
		unit = 50
	case price >= 10000:
		unit = 10
	case price >= 1000:
		unit = 1
	case price >= 100:
		// "100 이상 1,000 미만": 보통 단위는 0.1원이지만, 아래 특별 종목인 경우 1원 단위
		if isSpecialPair(pair) {
			unit = 1
		} else {
			unit = 0.1
		}
	case price >= 10:
		unit = 0.01
	case price >= 1:
		unit = 0.001
	case price >= 0.1:
		unit = 0.0001
	case price >= 0.01:
		unit = 0.00001
	case price >= 0.001:
		unit = 0.000001
	case price >= 0.0001:
		unit = 0.0000001
	default:
		unit = 0.00000001
	}

	// 내림 처리: price - (price % unit)
	return price - math.Mod(price, unit)
}

// isSpecialPair는 주문 가격 단위가 1원으로 적용되어야 하는 원화 마켓 종목인지 확인합니다.
// Upbit에서 거래쌍은 "KRW-XXX" 형식이므로, base 통화(XXX)를 기준으로 판별합니다.
func isSpecialPair(pair string) bool {
	parts := strings.Split(pair, "-")
	if len(parts) < 2 {
		return false
	}
	base := parts[1]
	// 특별 종목 목록 (2024.10.14부터 1원 단위 적용)
	special := map[string]bool{
		"ADA":  true,
		"ALGO": true,
		"BLUR": true,
		"CELO": true,
		"ELF":  true,
		"EOS":  true,
		"GRS":  true,
		"GRT":  true,
		"ICX":  true,
		"MANA": true,
		"MINA": true,
		"POL":  true,
		"SAND": true,
		"SEI":  true,
		"STG":  true,
		"TRX":  true,
	}
	return special[base]
}

func (u *Upbit) getFeeRateForOrder(pair string, side model.SideType, discountEvent bool) (float64, error) {
	baseFeeRate := 0.00139
	if side == model.SideTypeBuy {
		if discountEvent {
			return 0.0005, nil
		}
		return baseFeeRate, nil
	}
	// 매도 주문은 보통 체결 금액에서 수수료가 차감되므로, 주문 전 계산에는 별도 조정이 필요없습니다.
	return 0, nil
}
