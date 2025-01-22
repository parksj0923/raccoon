package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	upbitPrivateWS = "wss://api.upbit.com/websocket/v1/private"
	RandomWsUuid   = "0a9b8f1e-0dd9-4a59-adf8-0b7c10008943"
	Candle1s       = "candle.1s"

	Ticker    = "ticker"
	Trade     = "trade"
	OrderBook = "orderbook"
	MyOrder   = "myOrder"
	MyAsset   = "myAsset"
)

type Upbit struct {
	ctx        context.Context
	cancelFunc context.CancelFunc

	// REST
	apiKey    string
	secretKey string
	resty     resty.RestyClient

	// WebSocket
	wsConn    *websocket.Conn
	wsRunning bool
	wsMtx     sync.Mutex

	// HeikinAshi, MetadataFetchers
	HeikinAshi       bool
	MetadataFetchers []MetadataFetchers

	// 실시간 봉(Candle) 전송 채널 + 에러 채널 + 로직 제어
	assetsInfo map[string]model.AssetInfo
	candleCh   chan model.Candle
	errCh      chan error

	// Upbit에서 실시간 'candle.1s'을 받고, 1분봉으로 합성
	aggregatorMap map[string]*CandleAggregator
}

// CandleAggregator : 특정 pair(예: KRW-BTC)에 대한 실시간 봉 생성기
// 1초봉들(WS) 누적 → 1분마다 종가 확정
type CandleAggregator struct {
	pair       string
	buffer     []model.Candle
	currentMin time.Time
}

// MetadataFetchers, UpbitOption
type MetadataFetchers func(pair string, t time.Time) (string, float64)
type UpbitOption func(*Upbit)

func WithUpbitHeikinAshiCandle() UpbitOption {
	return func(u *Upbit) {
		u.HeikinAshi = true
	}
}
func WithMetadataFetcher(fetcher MetadataFetchers) UpbitOption {
	return func(u *Upbit) {
		u.MetadataFetchers = append(u.MetadataFetchers, fetcher)
	}
}

// NewUpbit : Upbit 객체 생성
func NewUpbit(apiKey, secretKey string, pairs []string, opts ...UpbitOption) (*Upbit, error) {
	//pair => "KRW-BTC"
	ctx, cancel := context.WithCancel(context.Background())
	restyClient := resty.NewDefaultRestyClient(true, 10*time.Second)
	up := &Upbit{
		ctx:           ctx,
		resty:         restyClient,
		cancelFunc:    cancel,
		apiKey:        apiKey,
		secretKey:     secretKey,
		candleCh:      make(chan model.Candle),
		errCh:         make(chan error),
		aggregatorMap: make(map[string]*CandleAggregator),
	}
	for _, opt := range opts {
		opt(up)
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
	params := map[string]string{}
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
	params := map[string]string{
		"market":   pair,
		"states[]": "wait",
		"limit":    strconv.Itoa(limit),
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
	params := map[string]string{
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
		params := map[string]string{
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
		params := map[string]string{
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

	params := map[string]string{
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
	params := map[string]string{}
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
func (u *Upbit) LastQuote(ctx context.Context, pair string) (float64, error) {
	// GET /v1/ticker?markets=KRW-BTC
	q := url.Values{}
	q.Set("markets", pair)
	body, err := utils.SimpleGet(ctx, upbitBaseREST+"/v1/ticker?"+q.Encode())
	if err != nil {
		return 0, err
	}
	var res []struct {
		TradePrice float64 `json:"trade_price"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return 0, err
	}
	if len(res) < 1 {
		return 0, fmt.Errorf("no ticker data for %s", pair)
	}
	return res[0].TradePrice, nil
}

// CandlesByLimit : (REST) /v1/candles/...
func (u *Upbit) CandlesByLimit(ctx context.Context, pair, period string, limit int) ([]model.Candle, error) {
	// TODO: upbitCandles, reverse, heikinAshi...
	return nil, errors.New("not implemented: CandlesByLimit upbit REST")
}

// CandlesByPeriod : (REST) start~end
func (u *Upbit) CandlesByPeriod(ctx context.Context, pair, period string,
	start, end time.Time) ([]model.Candle, error) {
	return nil, errors.New("not implemented: CandlesByPeriod upbit REST")
}

// CandlesSubscription : WebSocket 실시간 캔들
//
// 1) Upbit는 "type": "candle.1s"로 요청하면 "1초봉" 실시간 데이터 받을 수 있음
// 2) ninjabot "OnCandle"로는 1분봉,5분봉 등 원하는 TF가 필요
// -> 여기서는 예시로 candle.1s를 받은 뒤, aggregator에서 1분 단위 봉으로 변환해 전달
func (u *Upbit) CandlesSubscription(ctx context.Context, pair, timeframe string) (chan model.Candle, chan error) {
	// 우선 aggregatorMap에 pair용 aggregator 생성
	u.aggregatorMap[pair] = &CandleAggregator{
		pair: pair,
	}

	// goroutine에서 ws 연결
	// timeframe=="1m"라면 candle.1s로 받아서 1분봉 합성
	// 실제로 "candle.5s" 같은 건 문서상 없음. "candle.1s"만 공식
	go u.wsRunIfNeeded()
	return u.candleCh, u.errCh
}

// -----------------------------------------------------------------------------
// 실시간 캔들 Aggregator: "1초봉" -> "1분봉"
// -----------------------------------------------------------------------------

// handleUpbitCandle1s : upbit "candle.1s" 메시지를 받아 model.Candle로 변환
// 그리고 aggregator에 push
func (u *Upbit) handleUpbitCandle1s(msg []byte) {
	var raw struct {
		Type              string  `json:"type"` // candle.1s
		Code              string  `json:"code"`
		CandleDateTimeUTC string  `json:"candle_date_time_utc"`
		OpeningPrice      float64 `json:"opening_price"`
		HighPrice         float64 `json:"high_price"`
		LowPrice          float64 `json:"low_price"`
		TradePrice        float64 `json:"trade_price"`
		CandleAccVolume   float64 `json:"candle_acc_trade_volume"`
		CandleAccPrice    float64 `json:"candle_acc_trade_price"`
		Timestamp         int64   `json:"timestamp"`
		StreamType        string  `json:"stream_type"`
	}
	if err := json.Unmarshal(msg, &raw); err != nil {
		log.Errorf("handleUpbitCandle1s unmarshal: %v", err)
		return
	}
	t, _ := time.Parse("2006-01-02T15:04:05", raw.CandleDateTimeUTC)

	c := model.Candle{
		Pair:      raw.Code,
		Time:      t,
		UpdatedAt: t,
		Open:      raw.OpeningPrice,
		High:      raw.HighPrice,
		Low:       raw.LowPrice,
		Close:     raw.TradePrice,
		Volume:    raw.CandleAccVolume,
		Complete:  true, // 1초봉은 이미 완료 상태
		Metadata:  map[string]float64{},
	}
	// aggregator에 push
	agg, ok := u.aggregatorMap[raw.Code]
	if ok {
		newCandle, finished := agg.Push1sCandle(c)
		if finished {
			// 완성된 1분봉
			if u.HeikinAshi {
				newCandle = newCandle.ToHeikinAshi(nil)
			}
			for _, fetcher := range u.MetadataFetchers {
				k, v := fetcher(newCandle.Pair, newCandle.Time)
				newCandle.Metadata[k] = v
			}
			// 전달
			u.candleCh <- newCandle
		}
	}
}

// Push1sCandle : 1초봉을 누적하여 1분봉이 완성됐는지 체크
// 간단히 “timestamp의 분이 바뀔 때” 봉 완성 -> 새 봉
func (agg *CandleAggregator) Push1sCandle(c model.Candle) (model.Candle, bool) {
	if agg.currentMin.IsZero() {
		// 첫 봉
		agg.currentMin = c.Time.Truncate(time.Minute)
	}
	thisMinute := c.Time.Truncate(time.Minute)
	if thisMinute.After(agg.currentMin) {
		// 이전 분봉 완료
		finishedCandle := agg.aggregateBuffer()
		// 다음 분으로 넘어감
		agg.buffer = []model.Candle{}
		agg.currentMin = thisMinute
		// 새 1초봉을 buffer에 추가
		agg.buffer = append(agg.buffer, c)
		return finishedCandle, true
	} else {
		// 같은 분이면 buffer에 쌓기
		agg.buffer = append(agg.buffer, c)
		return model.Candle{}, false
	}
}

// aggregateBuffer : agg.buffer 내 모든 1초봉을 합쳐서 1분봉 생성
func (agg *CandleAggregator) aggregateBuffer() model.Candle {
	// 시간이 같은 애들(예: 1분간 60개 최대) => min/avg ...
	if len(agg.buffer) == 0 {
		// 빈 버퍼라면 dummy
		return model.Candle{}
	}
	first := agg.buffer[0]
	c := model.Candle{
		Pair:      first.Pair,
		Time:      agg.currentMin, // 1분봉 시각
		UpdatedAt: agg.currentMin,
		Open:      first.Open,
		High:      first.High,
		Low:       first.Low,
		Close:     first.Close,
		Volume:    0,
		Complete:  true,
		Metadata:  make(map[string]float64),
	}
	for i, sub := range agg.buffer {
		if i > 0 {
			if sub.High > c.High {
				c.High = sub.High
			}
			if sub.Low < c.Low {
				c.Low = sub.Low
			}
		}
		c.Close = sub.Close
		c.Volume += sub.Volume
	}
	return c
}

// -----------------------------------------------------------------------------
// WebSocket run (candle.1s 구독)
// -----------------------------------------------------------------------------

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
	}()

	// 연결
	conn, _, err := websocket.DefaultDialer.Dial(upbitBaseWS, nil)
	if err != nil {
		u.errCh <- fmt.Errorf("websocket dial fail: %w", err)
		return
	}
	u.wsConn = conn
	log.Info("[UpbitWS] connected")

	// example: candle.1s 구독 (pair list from aggregatorMap)
	var codes []string
	for p := range u.aggregatorMap {
		// 반드시 대문자 "KRW-BTC"
		codes = append(codes, strings.ToUpper(p))
	}

	subMsg := []interface{}{
		map[string]string{"ticket": "ninjabot-upbit-candle"},
		map[string]interface{}{
			"type":  "candle.1s",
			"codes": codes,
		},
		map[string]string{"format": "DEFAULT"},
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		u.errCh <- fmt.Errorf("websocket write subscription fail: %w", err)
		return
	}

	// read loop
	for {
		select {
		case <-u.ctx.Done():
			log.Info("[UpbitWS] context canceled, closing ws")
			conn.Close()
			return
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
				u.errCh <- fmt.Errorf("websocket read fail: %w", err)
				return
			}
			// parse candle.1s
			// 응답이 "type": "candle.1s" 인 경우만 처리
			var raw struct {
				Type  string `json:"type"`
				Error struct {
					Name    string `json:"name"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if e := json.Unmarshal(msg, &raw); e != nil {
				log.Warnf("[UpbitWS] unmarshal raw fail: %v", e)
				continue
			}
			if raw.Error.Name != "" {
				// Upbit WS error
				u.errCh <- fmt.Errorf("UpbitWS error: %s - %s", raw.Error.Name, raw.Error.Message)
				continue
			}
			if raw.Type == "candle.1s" {
				u.handleUpbitCandle1s(msg)
			} else {
				// 다른 타입은 무시 (ticker, trade, etc.)
			}
		}
	}
}

func (u *Upbit) Stop() {
	// ninjabot에는 별도로 Stop()이 없을 수도 있으나, 필요 시 제공
	u.cancelFunc()
	if u.wsConn != nil {
		u.wsConn.Close()
	}
	close(u.candleCh)
	close(u.errCh)
	log.Info("[Upbit] stopped")
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
		t, err := time.Parse(time.RFC3339, o.CreatedAt)
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
			t, err := time.Parse(time.RFC3339, o.CreatedAt)
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
func (u *Upbit) requestUpbitGET(ctx context.Context, path string, params map[string]string) ([]byte, error) {
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
	if resp.StatusCode() != 201 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// requestUpbitPOST : Upbit JWT + POST
func (u *Upbit) requestUpbitPOST(ctx context.Context, path string, params map[string]string) ([]byte, error) {
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
	if resp.StatusCode() != 201 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// requestUpbitDELETE
func (u *Upbit) requestUpbitDELETE(ctx context.Context, path string, params map[string]string) ([]byte, error) {
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
	if resp.StatusCode() != 201 {
		return nil, fmt.Errorf("API 응답 오류: %d, %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}

// floatToString
func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (u *Upbit) fetchChance(pair string) (*model.OrderChanceResponse, error) {
	params := map[string]string{
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
	quote := ch.Market.Ask.Currency
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
