// chartview/store.go
package chartview

import (
	"raccoon/model"
	"sort"
	"sync"
)

// ChartDataStore : 봉(Candle) + 지표(RSI, MACD 등)를 저장
type ChartDataStore struct {
	mu sync.Mutex

	// 실시간으로 추가되는 완성봉
	candles []model.Candle

	// 지표 예시: rsi, macd, macdSignal, macdHist
	rsi14      []float64
	macd       []float64
	macdSignal []float64
	macdHist   []float64
}

// GlobalChartData : 전역(싱글톤) 데이터 저장소
var GlobalChartData = &ChartDataStore{}

// AppendCandle : 신규 완성봉 저장
func (ds *ChartDataStore) AppendCandle(candle model.Candle) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.candles = append(ds.candles, candle)
}

// GetCandles : 현재 저장된 모든 봉 복사 반환
func (ds *ChartDataStore) GetCandles() []model.Candle {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	out := make([]model.Candle, len(ds.candles))
	copy(out, ds.candles)

	// 시간 오름차순 정렬
	sort.Slice(out, func(i, j int) bool {
		return out[i].Time.Before(out[j].Time)
	})
	return out
}

// UpdateIndicators : 전략에서 계산된 지표들을 반영
func (ds *ChartDataStore) UpdateIndicators(rsi, macd, macdSig, hist []float64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.rsi14 = rsi
	ds.macd = macd
	ds.macdSignal = macdSig
	ds.macdHist = hist
}

// GetIndicators : 지표 배열 4종 반환
func (ds *ChartDataStore) GetIndicators() (rsi, macd, macdSig, hist []float64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	return ds.rsi14, ds.macd, ds.macdSignal, ds.macdHist
}

// GetTimeAxis : x축(time)을 문자열 배열로 변환
func (ds *ChartDataStore) GetTimeAxis() []string {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	out := make([]string, len(ds.candles))
	for i, c := range ds.candles {
		// 원하는 포맷 (날짜+시각 등)
		out[i] = c.Time.Format("01/02 15:04")
	}
	return out
}
