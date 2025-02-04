package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"raccoon/utils/log"
	"sync"
	"time"

	"raccoon/model"
)

// WebServer manages SSE clients and stores chart data.
type WebServer struct {
	mu           sync.RWMutex
	candlesticks []CandleData     // 캔들 데이터 기록
	indicators   []IndicatorEvent // 지표 이벤트 기록
	orders       []OrderEvent     // 주문 이벤트 기록

	sseClients map[chan []byte]bool
	sseMu      sync.Mutex
}

// CandleData: Chart.js Financial 플러그인은 시간 정보를 "x" 필드에 기대합니다.
type CandleData struct {
	X        int64   `json:"x"` // Unix 밀리초 timestamp
	O        float64 `json:"o"`
	H        float64 `json:"h"`
	L        float64 `json:"l"`
	C        float64 `json:"c"`
	Volume   float64 `json:"volume,omitempty"`
	Complete bool    `json:"complete,omitempty"`
}

type IndicatorValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type IndicatorEvent struct {
	Time       int64            `json:"time"`
	Indicators []IndicatorValue `json:"indicators"`
}

type OrderEvent struct {
	Time  int64   `json:"time"`
	Pair  string  `json:"pair"`
	Side  string  `json:"side"`
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

func NewWebServer() *WebServer {
	return &WebServer{
		candlesticks: make([]CandleData, 0),
		indicators:   make([]IndicatorEvent, 0),
		orders:       make([]OrderEvent, 0),
		sseClients:   make(map[chan []byte]bool),
	}
}

func (ws *WebServer) OnCandle(candle model.Candle) {
	cd := CandleData{
		X:        candle.Time.UnixMilli(), // "x" 필드에 저장
		O:        candle.Open,
		H:        candle.High,
		L:        candle.Low,
		C:        candle.Close,
		Volume:   candle.Volume,
		Complete: candle.Complete,
	}
	ws.mu.Lock()
	n := len(ws.candlesticks)
	if n > 0 && ws.candlesticks[n-1].X == cd.X {
		ws.candlesticks[n-1] = cd
	} else {
		ws.candlesticks = append(ws.candlesticks, cd)
	}
	ws.mu.Unlock()

	ws.broadcastSSE("candle", cd)
}

func (ws *WebServer) OnIndicators(ts time.Time, values []IndicatorValue) {
	evt := IndicatorEvent{
		Time:       ts.UnixMilli(),
		Indicators: values,
	}
	log.Infof("Broadcasting indicators event: %+v", evt)
	ws.mu.Lock()
	ws.indicators = append(ws.indicators, evt)
	ws.mu.Unlock()

	ws.broadcastSSE("indicators", evt)
}

func (ws *WebServer) OnOrder(order model.Order) {
	evt := OrderEvent{
		Time:  time.Now().UnixMilli(),
		Pair:  order.Pair,
		Side:  string(order.Side),
		Price: order.Price,
		Qty:   order.Quantity,
	}
	ws.mu.Lock()
	ws.orders = append(ws.orders, evt)
	ws.mu.Unlock()

	ws.broadcastSSE("order", evt)
}

func (ws *WebServer) broadcastSSE(typ string, data interface{}) {
	ws.sseMu.Lock()
	defer ws.sseMu.Unlock()

	payload, _ := json.Marshal(struct {
		Type string      `json:"type"`
		Data interface{} `json:"data"`
	}{
		Type: typ,
		Data: data,
	})

	for ch := range ws.sseClients {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (ws *WebServer) sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan []byte, 50)
	ws.sseMu.Lock()
	ws.sseClients[clientChan] = true
	ws.sseMu.Unlock()

	notify := r.Context().Done()
	go func() {
		<-notify
		ws.sseMu.Lock()
		delete(ws.sseClients, clientChan)
		close(clientChan)
		ws.sseMu.Unlock()
	}()

	ws.mu.RLock()
	// 저장된 모든 데이터(캔들, 지표, 주문)를 클라이언트로 전송
	for _, c := range ws.candlesticks {
		msg, _ := json.Marshal(struct {
			Type string     `json:"type"`
			Data CandleData `json:"data"`
		}{
			"candle", c,
		})
		fmt.Fprintf(w, "data: %s\n\n", string(msg))
	}
	for _, ind := range ws.indicators {
		msg, _ := json.Marshal(struct {
			Type string         `json:"type"`
			Data IndicatorEvent `json:"data"`
		}{
			"indicators", ind,
		})
		fmt.Fprintf(w, "data: %s\n\n", string(msg))
	}
	for _, od := range ws.orders {
		msg, _ := json.Marshal(struct {
			Type string     `json:"type"`
			Data OrderEvent `json:"data"`
		}{
			"order", od,
		})
		fmt.Fprintf(w, "data: %s\n\n", string(msg))
	}
	ws.mu.RUnlock()
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-clientChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
			flusher.Flush()
		}
	}
}

func (ws *WebServer) chartHandler(w http.ResponseWriter, r *http.Request) {
	// HTML + JavaScript 코드: 두 개의 캔버스(가격/지표용, 거래량용)를 사용
	html := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Mixed Chart: Candlestick, Indicators, Volume & Orders</title>
  <!-- Luxon, Chart.js, 어댑터, 그리고 Chart.js Financial 플러그인 로드 -->
  <script src="https://cdn.jsdelivr.net/npm/luxon@3.4.4"></script>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@3.9.1/dist/chart.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-luxon@1.3.1"></script>
  <script src="https://cdn.jsdelivr.net/npm/chartjs-chart-financial@0.2.1/dist/chartjs-chart-financial.js"></script>
  <style>
    /* 간단한 레이아웃: 위에 가격/지표 차트, 아래에 거래량 차트 */
    #charts {
      display: flex;
      flex-direction: column;
      align-items: center;
    }
    canvas {
      margin: 10px 0;
    }
  </style>
  <script>
    // 플러그인 등록 (Chart.js v3.x 기준)
    if (Chart.FinancialController && Chart.CandlestickElement && Chart.OhlcElement) {
      Chart.register(Chart.FinancialController, Chart.CandlestickElement, Chart.OhlcElement);
    } else {
      console.error("Chart.js Financial 관련 객체가 정의되지 않았습니다.");
    }

    // priceChart의 x축 범위를 재계산하는 함수 (candlestick 데이터 기준)
    function recalcPriceXScales(chart) {
      const ds = chart.data.datasets[0];
      if (!ds.data.length) return;
      ds.data.sort((a, b) => a.x - b.x);
      const margin = 30000; // 30초 margin
      const xMin = ds.data[0].x - margin;
      const xMax = ds.data[ds.data.length - 1].x + margin;
      chart.options.scales.x.min = xMin;
      chart.options.scales.x.max = xMax;
    }
  
    window.addEventListener('DOMContentLoaded', function() {
      // 두 개의 캔버스 요소 선택
      const priceCtx = document.getElementById('priceChart').getContext('2d');
      const volumeCtx = document.getElementById('volumeChart').getContext('2d');
      
      // 캔들/지표/주문 차트 (priceChart)
      const priceChart = new Chart(priceCtx, {
        data: {
          datasets: [
            {
              label: 'Price',
              type: 'candlestick',
              data: [],
              parsing: false,
              yAxisID: 'yCandles'
            },
            {
              label: 'Orders',
              type: 'scatter',
              data: [],
              yAxisID: 'yCandles',
              showLine: false,
              pointStyle: 'triangle',
              radius: 6
            }
          ]
        },
        options: {
          responsive: true,
          animation: false,
          scales: {
            x: { 
              type: 'time',
              time: {
                unit: 'minute',
                tooltipFormat: 'HH:mm:ss',
                displayFormats: {
                  minute: 'HH:mm'
                }
              }
            },
            yCandles: {
              position: 'left',
              beginAtZero: false,
              title: {
                display: true,
                text: 'Price'
              }
            }
            // indicator용 y축은 동적으로 추가됩니다.
          }
        }
      });

      // 거래량 차트 (volumeChart) - Bar 차트, 데이터는 {x, y} 객체 배열로 구성
      const volumeChart = new Chart(volumeCtx, {
        type: 'bar',
        data: {
          datasets: [{
            label: 'Volume',
            data: [],
            backgroundColor: 'rgba(0, 0, 255, 0.3)'
          }]
        },
        options: {
          responsive: true,
          animation: false,
          scales: {
            x: {
              type: 'time',
              time: {
                unit: 'minute',
                tooltipFormat: 'HH:mm:ss',
                displayFormats: {
                  minute: 'HH:mm'
                }
              }
            },
            y: {
              type: 'linear',
              beginAtZero: true,
              title: {
                display: true,
                text: 'Volume'
              }
              // y축 max는 동적으로 설정합니다.
            }
          }
        }
      });
  
      // SSE 이벤트 처리
      const evtSource = new EventSource('/sse');
      evtSource.onmessage = function(ev) {
        const parsed = JSON.parse(ev.data);
		console.log("Received SSE event:", parsed);
			if (parsed.type === 'indicators') {
				console.log("Indicator event data:", parsed.data);
			}
        switch(parsed.type) {
          case 'candle': {
            const c = parsed.data;
            // priceChart 업데이트 (candlestick)
            let dsPrice = priceChart.data.datasets[0];
            let idx = dsPrice.data.findIndex(item => item.x === c.x);
            if (idx >= 0) {
              dsPrice.data[idx] = c;
            } else {
              dsPrice.data.push(c);
            }
            recalcPriceXScales(priceChart);
            priceChart.update();

            // volumeChart 업데이트: dataset.data를 {x, y} 객체로 추가/업데이트
            let volDataset = volumeChart.data.datasets[0];
            let vIdx = volDataset.data.findIndex(item => item.x === c.x);
            // c.volume가 숫자형이 아닌 경우 Number()를 통해 변환
            const volumeValue = Number(c.volume);
            if (vIdx >= 0) {
              volDataset.data[vIdx].y = volumeValue;
            } else {
              volDataset.data.push({ x: c.x, y: volumeValue });
            }
            // 최대 거래량을 계산하여 y축 max를 설정
            let maxVol = 0;
            volDataset.data.forEach(item => {
              if (item.y > maxVol) { maxVol = item.y; }
            });
            volumeChart.options.scales.y.max = maxVol * 1.1;
            volumeChart.update();
            break;
          }
          case 'indicators': {
            const iEvt = parsed.data;
            const tVal = iEvt.time;
            iEvt.indicators.forEach(iv => {
              let ds = getOrCreateLineDataset(priceChart, iv.name);
              let found = ds.data.find(pt => pt.x === tVal);
              if(found) {
                found.y = iv.value;
              } else {
                ds.data.push({ x: tVal, y: iv.value });
              }
            });
            priceChart.update();
            break;
          }
          case 'order': {
            const dsOrder = priceChart.data.datasets[1];
            const od = parsed.data;
            let color = (od.side==="buy" || od.side==="bid") ? "green" : "red";
            dsOrder.data.push({ x: od.time, y: od.price, backgroundColor: color, borderColor: color });
            priceChart.update();
            break;
          }
          default:
            console.log("Unknown SSE event:", parsed);
        }
      };
  
      // getOrCreateLineDataset: indicator 이름에 따라 데이터셋과 고유 y축을 동적으로 생성 (priceChart에 추가)
      function getOrCreateLineDataset(chart, name) {
        const axisId = 'yIndicator_' + name;  // 고유 y축 ID
        let ds = chart.data.datasets.find(d => d.label === name);
        if (!ds) {
          ds = {
            label: name,
            type: 'line',
            data: [],
            borderColor: pickRandomColor(),
            fill: false,
            pointRadius: 1,
            yAxisID: axisId
          };
          chart.data.datasets.push(ds);
        }
        // 해당 y축이 없으면 동적으로 추가합니다.
        if (!chart.options.scales[axisId]) {
          chart.options.scales[axisId] = {
            position: 'right',
            beginAtZero: true,
            grid: {
              drawOnChartArea: false
            },
            title: {
              display: true,
              text: name
            }
          };
        }
        return ds;
      }
  
      function pickRandomColor() {
        let r = Math.floor(Math.random() * 256);
        let g = Math.floor(Math.random() * 256);
        let b = Math.floor(Math.random() * 256);
        return "rgb(" + r + "," + g + "," + b + ")";
      }
    });
  </script>
</head>
<body>
  <h1>Mixed Chart: Candlestick + Indicators + Volume & Orders</h1>
  <div id="charts">
    <canvas id="priceChart" width="1200" height="400"></canvas>
    <canvas id="volumeChart" width="1200" height="150"></canvas>
  </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (ws *WebServer) Start(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/chart", ws.chartHandler)
	mux.HandleFunc("/sse", ws.sseHandler)
	fmt.Println("[WebServer] Listening on", port)
	return http.ListenAndServe(port, mux)
}
