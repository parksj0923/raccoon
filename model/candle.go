package model

import "time"

type Candle struct {
	Pair      string    `json:"pair,omitempty"`
	Time      time.Time `json:"time"`
	UpdatedAt time.Time `json:"updatedAt"`
	Open      float64   `json:"open"`
	Close     float64   `json:"close"`
	Low       float64   `json:"low"`
	High      float64   `json:"high"`
	Volume    float64   `json:"volume"`
	Complete  bool      `json:"complete"`

	// Aditional collums from CSV inputs
	Metadata map[string]float64 `json:"metadata,omitempty"`
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
