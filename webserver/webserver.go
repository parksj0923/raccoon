package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"raccoon/model"
)

// WebServer manages SSE clients and stores chart data.
type WebServer struct {
	mu           sync.RWMutex
	candlesticks []CandleData     // candlestick records
	indicators   []IndicatorEvent // indicator records
	orders       []OrderEvent     // order records

	sseClients map[chan []byte]bool
	sseMu      sync.Mutex
}

// CandleData: Chart.js financial plugin expects the time field as "t".
type CandleData struct {
	T        int64   `json:"t"` // 변경: "x" → "t"
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
		T:        candle.Time.UnixMilli(), // 시간 필드를 T에 저장
		O:        candle.Open,
		H:        candle.High,
		L:        candle.Low,
		C:        candle.Close,
		Volume:   candle.Volume,
		Complete: candle.Complete,
	}
	ws.mu.Lock()
	n := len(ws.candlesticks)
	if n > 0 && ws.candlesticks[n-1].T == cd.T {
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
	// HTML + JS 코드: Chart.js, chartjs-chart-financial, Luxon, 그리고 SSE 연결.
	// x축은 'time' 타입으로 설정하고, 시간 단위 및 포맷을 Luxon 어댑터에 맞게 지정합니다.
	// 또한 recalcScales() 함수로 x축, y축 범위를 동적으로 재계산합니다.
	html := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Mixed Chart: Candlestick + Indicators + Orders</title>
  <!-- Luxon, Chart.js, 그리고 어댑터/플러그인 로드 -->
  <script src="https://cdn.jsdelivr.net/npm/luxon@3.4.4"></script>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-luxon@1.3.1"></script>
  <script src="https://cdn.jsdelivr.net/npm/chartjs-chart-financial@0.2.1/dist/chartjs-chart-financial.js"></script>
  <script>
    // recalcScales: 데이터셋을 시간순으로 정렬하고 x축, y축 범위를 동적으로 재계산합니다.
    function recalcScales(chart) {
      const ds = chart.data.datasets[0];
      if (!ds.data.length) return;
      ds.data.sort((a, b) => a.t - b.t);
      const margin = 30000; // 30초 margin
      const xMin = ds.data[0].t - margin;
      const xMax = ds.data[ds.data.length - 1].t + margin;
      let yMin = Infinity, yMax = -Infinity;
      ds.data.forEach(point => {
        if (point.l < yMin) yMin = point.l;
        if (point.h > yMax) yMax = point.h;
      });
      chart.options.scales.x.min = xMin;
      chart.options.scales.x.max = xMax;
      chart.options.scales.yCandles.min = yMin;
      chart.options.scales.yCandles.max = yMax;
    }
  
    window.addEventListener('DOMContentLoaded', function() {
      const ctx = document.getElementById('mixedChart').getContext('2d');
      
      // (필요시 Chart.register를 호출하여 financial 컨트롤러와 요소를 등록)
      // Chart.register(Chart.FinancialController, Chart.CandlestickElement, Chart.OhlcElement);
      
      const mixedChart = new Chart(ctx, {
        data: {
          datasets: [
            {
              label: 'Price',
              type: 'candlestick',
              data: [],
              parsing: false, // 이미 파싱된 데이터 사용
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
              beginAtZero: false
            }
          }
        }
      });
  
      const evtSource = new EventSource('/sse');
      evtSource.onmessage = function(ev) {
        const parsed = JSON.parse(ev.data);
        switch(parsed.type) {
          case 'candle': {
            const dsCandle = mixedChart.data.datasets[0];
            const c = parsed.data;
            let idx = dsCandle.data.findIndex(item => item.t === c.t);
            if (idx >= 0) {
              dsCandle.data[idx] = c;
            } else {
              dsCandle.data.push(c);
            }
            recalcScales(mixedChart);
            console.log("Dataset:", dsCandle.data);
            console.log("x-axis range:", mixedChart.options.scales.x.min, mixedChart.options.scales.x.max);
            mixedChart.update();
            break;
          }
          case 'indicators': {
            const iEvt = parsed.data;
            let tVal = iEvt.time;
            iEvt.indicators.forEach(iv => {
              let ds = getOrCreateLineDataset(mixedChart, iv.name);
              let found = ds.data.find(pt => pt.x === tVal);
              if(found) {
                found.y = iv.value;
              } else {
                ds.data.push({ x: tVal, y: iv.value });
              }
            });
            mixedChart.update();
            break;
          }
          case 'order': {
            const dsOrder = mixedChart.data.datasets[1];
            const od = parsed.data;
            let color = (od.side==="buy" || od.side==="bid") ? "green" : "red";
            dsOrder.data.push({ x: od.time, y: od.price, backgroundColor: color, borderColor: color });
            mixedChart.update();
            break;
          }
          default:
            console.log("Unknown SSE:", parsed.type, parsed.data);
        }
      };
  
      function getOrCreateLineDataset(chart, name) {
        let ds = chart.data.datasets.find(d => d.label === name);
        if (!ds) {
          ds = {
            label: name,
            type: 'line',
            data: [],
            borderColor: pickRandomColor(),
            fill: false,
            pointRadius: 1,
            yAxisID: 'yCandles'
          };
          chart.data.datasets.push(ds);
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
  <h1>Mixed Chart: Candlestick + Indicators + Orders</h1>
  <canvas id="mixedChart" width="1200" height="600"></canvas>
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
