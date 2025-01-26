package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"raccoon/utils/collection"
	"raccoon/utils/resty"
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

// CandleAggregator : 특정 pair(예: KRW-BTC)에 대한 실시간 봉 생성기
// 1초봉들(WS) 누적 → 원하는 period 에 맞게 합성
type CandleAggregator struct {
	pair     string
	period   string
	duration time.Duration

	//1초당 들어오는 candle을 임시저장, 매초를 key로 잡아 중복데이터지만 다른시간인경우 override
	buffer map[time.Time]model.Candle
	//현재 집계중인 시간 구간의 시작 시각을 저장
	currentKey time.Time

	candleCh chan model.Candle
	errCh    chan error
}

// NewUpbit : Upbit 객체 생성
func NewUpbit(apiKey, secretKey string, pairs []string) (*Upbit, error) {
	//pair => "KRW-BTC"
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
		chance, err := up.fetchChance(pair)
		if err != nil {
			log.Errorf("[UPBIT] Failed to fetch upbit exchange pair %s: %v", pair, err)
			continue
		}
		// chance -> model.AssetInfo
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

// -----------------------------------------------------------------------------
// Broker 구현부: 주문, 계좌 등
// -----------------------------------------------------------------------------

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

func (u *Upbit) Position(pair string) (asset, quote float64, err error) {
	acc, err := u.Account()
	if err != nil {
		return 0, 0, err
	}
	base, quoteAsset := SplitAssetQuote(pair)
	var baseBal, quoteBal float64
	for _, b := range acc.Balances {
		if strings.EqualFold(b.Currency, base) {
			baseBal = b.Balance + b.Locked
		}
		if strings.EqualFold(b.Currency, quoteAsset) {
			quoteBal = b.Balance + b.Locked
		}
	}
	return baseBal, quoteBal, nil
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
	// Upbit: ord_type=limit, side=(bid|ask), price=limit, volume=quantity, tif(optional)=(ioc|fok)
	params := map[string]interface{}{
		"market":   pair,
		"side":     string(side),
		"ord_type": string(model.OrderTypeLimit),
		"price":    floatToString(limit),
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
		params := map[string]interface{}{
			"market":   pair,
			"side":     string(side),
			"ord_type": string(model.OrderTypePrice),
			"price":    floatToString(quantity),
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

// -----------------------------------------------------------------------------
// Feeder 구현부: 시세(캔들)
// -----------------------------------------------------------------------------

func (u *Upbit) AssetsInfo(pair string) model.AssetInfo {

	pair = strings.ToUpper(pair)
	if info, ok := u.assetsInfo[pair]; ok {
		return info
	}
	resp, err := u.fetchChance(pair)
	if err != nil {
		return model.AssetInfo{}
	}
	result, err := convertChanceToAssetInfo(resp)
	if err != nil {
		return model.AssetInfo{}
	}
	return result
}

// LastQuote : Ticker로 현재가
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

// CandlesByLimit : (REST) /v1/candles/...
func (u *Upbit) CandlesByLimit(pair, period string, limit int) ([]model.Candle, error) {
	if limit > CandlePageLimit {
		return nil, fmt.Errorf("candles limit exceeds 200")
	}
	// 1) period 파싱 -> Upbit candles endpoint
	endpoint, err := mapPeriodToCandleEndpoint(period)
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

// CandlesByPeriod : (REST) start~end
func (u *Upbit) CandlesByPeriod(pair, period string, start, end time.Time) ([]model.Candle, error) {
	endpoint, err := mapPeriodToCandleEndpoint(period)
	if err != nil {
		return nil, err
	}

	var allCandles []model.Candle

	// KST Location
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return nil, fmt.Errorf("fail to load KST loc: %w", err)
	}

	toTime := end.In(loc)

	for {
		toStr := toTime.Format("2006-01-02 15:04:05") + "+09:00"
		params := map[string]interface{}{
			"market": pair,
			"count":  CandlePageLimit,
			"to":     toStr,
		}

		body, err := u.requestUpbitGET(u.ctx, endpoint, params)
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

		// (f) 가장 오래된 캔들(=raw 마지막)에 적힌 시간을 구해, toTime = 그 시간보다 1초 더 과거
		// raw는 최신->과거 => 가장 오래된 것은 raw[len(raw)-1]
		oldest := raw[len(raw)-1]
		oldestTime, _ := time.ParseInLocation("2006-01-02T15:04:05", oldest.CandleDateTimeKST, KSTLocation)

		if !oldestTime.After(start) {
			break
		}
		toTime = oldestTime
	}

	collection.Sort(allCandles, func(a, b model.Candle) bool {
		return a.Time.Unix() < b.Time.Unix()
	})

	// start~end 범위 필터
	var result []model.Candle
	for _, c := range allCandles {
		if c.Time.Equal(start) || c.Time.Equal(end) ||
			(c.Time.After(start) && c.Time.Before(end)) {
			result = append(result, c)
		}
	}
	return result, nil
}

// CandlesSubscription : WebSocket 실시간 캔들
func (u *Upbit) CandlesSubscription(pair, period string) (chan model.Candle, chan error) {
	key := pair + "_" + period
	if agg, ok := u.aggregatorMap[key]; ok {
		return agg.candleCh, agg.errCh
	}
	dur, err := parseTimeframeToDuration(period)
	if err != nil {
		//TODO error 처리를 어떻게 제대로 할지
		cch := make(chan model.Candle)
		ech := make(chan error, 1)
		ech <- fmt.Errorf("unsupported timeframe: %s", period)
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

	go u.wsRunIfNeeded()
	return agg.candleCh, agg.errCh
}

// -----------------------------------------------------------------------------
// WebSocket run (candle.1s 구독)
// -----------------------------------------------------------------------------

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
					goto connect
				} else {
					u.broadcastErr(fmt.Errorf("read fail after %d retries: %w", maxWSRetries, err))
					conn.Close()
					return
				}
			}
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
			fin, done := agg.push1sCandle(candle)
			if done && fin.Volume > 0 {
				agg.candleCh <- fin
			}
		}
	}
}

// push1sCandle : 1초봉 -> (완성봉, bool)
func (agg *CandleAggregator) push1sCandle(c model.Candle) (model.Candle, bool) {
	// "1s" => 그냥 반환
	if agg.duration == 0 {
		agg.buffer[c.Time] = c
		return c, true
	}

	// 매초 buffer에 저장하고, 동일한 pair의 같은 시간의 데이터가 들어오는것은 최신데이터로 override
	agg.buffer[c.Time] = c

	thisKey := c.Time.Truncate(agg.duration)
	if agg.currentKey.IsZero() {
		agg.currentKey = thisKey
	}
	if thisKey.After(agg.currentKey) {
		// finish old interval
		fin := agg.aggregateBuffer(agg.currentKey)
		// remove old seconds
		agg.removeOldSeconds(agg.currentKey)
		agg.currentKey = thisKey
		return fin, true
	}
	return model.Candle{}, false
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

// aggregateBuffer : gather all seconds in [minKey, minKey+duration)
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
		return model.Candle{}
	}
	// sort by sc.Time
	collection.Sort(secs, func(i, j model.Candle) bool {
		return i.Time.Before(j.Time)
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
		Complete:  true,
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
	if resp.StatusCode() != 200 {
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
	if resp.StatusCode() != 200 {
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
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// floatToString
func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (u *Upbit) fetchChance(pair string) (*model.OrderChanceResponse, error) {
	params := map[string]interface{}{
		"market": pair,
	}
	body, err := u.requestUpbitGET(u.ctx, "/v1/orders/chance", params)
	if err != nil {
		return nil, err
	}
	var resp model.OrderChanceResponse
	if e := json.Unmarshal(body, &resp); e != nil {
		return nil, e
	}
	if resp.Market.ID == "" {
		return nil, errors.New("invalid chance response")
	}
	return &resp, nil
}

func convertChanceToAssetInfo(ch *model.OrderChanceResponse) (model.AssetInfo, error) {
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

func mapPeriodToCandleEndpoint(period string) (string, error) {
	switch period {
	case "1s":
		return "seconds", nil
	case "1m":
		return "minutes/1", nil
	case "3m":
		return "minutes/3", nil
	case "5m":
		return "minutes/5", nil
	case "10m":
		return "minutes/10", nil
	case "15m":
		return "minutes/15", nil
	case "30m":
		return "minutes/30", nil
	case "60m", "1h":
		return "minutes/60", nil
	case "240m":
		return "minutes/240", nil
	case "1d":
		return "days", nil
	case "1w":
		return "weeks", nil
	case "1M":
		return "months", nil
	case "1y":
		return "years", nil
	default:
		return "", fmt.Errorf("unsupported upbit period: %s", period)
	}
}

func parseTimeframeToDuration(tf string) (time.Duration, error) {
	switch tf {
	case "1s":
		return 0, nil
	case "1m":
		return time.Minute, nil
	case "3m":
		return 3 * time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "10":
		return 10 * time.Second, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "60m", "1h":
		return time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "1w":
		return 7 * 24 * time.Hour, nil
	case "1M":
		return 30 * 24 * time.Hour, nil
	case "1y":
		return 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported timeframe: %s", tf)
	}
}
