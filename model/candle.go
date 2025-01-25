package model

import "time"

type Candle struct {
	Pair      string
	Time      time.Time
	UpdatedAt time.Time
	Open      float64
	Close     float64
	Low       float64
	High      float64
	Volume    float64
	Complete  bool

	// Aditional collums from CSV inputs
	Metadata map[string]float64
}

type WSCandleBase struct {
	Type  string `json:"type"`
	Error struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"error"`
}

type WSCandle struct {
	Code                 string  `json:"code"`
	CandleDateTimeUtc    string  `json:"candle_date_time_utc"`
	CandleDateTimeKst    string  `json:"candle_date_time_kst"`
	OpeningPrice         float64 `json:"opening_price"`
	HighPrice            float64 `json:"high_price"`
	LowPrice             float64 `json:"low_price"`
	TradePrice           float64 `json:"trade_price"`
	CandleAccTradeVolume float64 `json:"candle_acc_trade_volume"`
	CandleAccTradePrice  float64 `json:"candle_acc_trade_price"`
	Timestamp            int64   `json:"timestamp"`
	StreamType           string  `json:"stream_type"`
}
