package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"raccoon/model"
)

// WebServer : SSE + 차트 데이터
type WebServer struct {
	mu sync.RWMutex

	candlesticks []CandleData     // 캔들(부분+완성) 기록
	indicators   []IndicatorEvent // 지표 기록
	orders       []OrderEvent     // 주문 기록

	sseClients map[chan []byte]bool
	sseMu      sync.Mutex
}

// CandleData : 캔들 OHLC 형식
type CandleData struct {
	X int64   `json:"x"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`

	Volume   float64 `json:"volume,omitempty"`
	Complete bool    `json:"complete,omitempty"`
}

// IndicatorValue : 단일 지표 이름/값
type IndicatorValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// IndicatorEvent : 시점 + 여러 지표
type IndicatorEvent struct {
	Time       int64            `json:"time"`
	Indicators []IndicatorValue `json:"indicators"`
}

// OrderEvent : 매수/매도 주문
type OrderEvent struct {
	Time  int64   `json:"time"`
	Pair  string  `json:"pair"`
	Side  string  `json:"side"`
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

// NewWebServer : 생성자
func NewWebServer() *WebServer {
	return &WebServer{
		candlesticks: []CandleData{},
		indicators:   []IndicatorEvent{},
		orders:       []OrderEvent{},
		sseClients:   make(map[chan []byte]bool),
	}
}

// OnCandle : 봉(부분+완성) => SSE
func (ws *WebServer) OnCandle(candle model.Candle) {
	cd := CandleData{
		X:        candle.Time.UnixMilli(),
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
		// 이미 해당 timestamp 있으니 갱신(부분봉->완성봉)
		ws.candlesticks[n-1] = cd
	} else {
		ws.candlesticks = append(ws.candlesticks, cd)
	}
	ws.mu.Unlock()

	ws.broadcastSSE("candle", cd)
}

// OnIndicators : 여러 지표 => SSE
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

// OnOrder : 주문 => SSE
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

// broadcastSSE : 모든 SSE 클라이언트에게 이벤트 전송
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

// sseHandler : /sse
func (ws *WebServer) sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	f, ok := w.(http.Flusher)
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

	// 초기 전송
	ws.mu.RLock()
	// (A) candlesticks
	for _, c := range ws.candlesticks {
		msg, _ := json.Marshal(struct {
			Type string     `json:"type"`
			Data CandleData `json:"data"`
		}{
			"candle", c,
		})
		fmt.Fprintf(w, "data: %s\n\n", string(msg))
	}
	// (B) indicators
	for _, ind := range ws.indicators {
		msg, _ := json.Marshal(struct {
			Type string         `json:"type"`
			Data IndicatorEvent `json:"data"`
		}{
			"indicators", ind,
		})
		fmt.Fprintf(w, "data: %s\n\n", string(msg))
	}
	// (C) orders
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
	f.Flush()

	// 새 이벤트는 clientChan 통해
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-clientChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
			f.Flush()
		}
	}
}

// chartHandler : /chart
// 하나의 Chart에 candlestick + line(지표) + scatter(주문) => mixed
func (ws *WebServer) chartHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>One Chart: Candlestick + Indicators + Orders</title>

  <!-- Chart.js -->
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.js"></script>

  <!-- chartjs-chart-financial -->
  <script src="https://cdn.jsdelivr.net/npm/chartjs-chart-financial@0.2.1/dist/chartjs-chart-financial.js"></script>

  <!-- Luxon + adapter -->
  <script src="https://cdn.jsdelivr.net/npm/luxon@3.4.4"></script>
  <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-luxon@1.3.1"></script>

  <script>
    window.addEventListener('DOMContentLoaded', function() {
  // (1) 라이브러리 존재 여부 확인 (전역 변수 이름 변경)
  if (window.ChartFinancial) {
    const { CandlestickController, OhlcController } = window.ChartFinancial;
    Chart.register(CandlestickController, OhlcController);
  } else {
    console.error('chartjs-chart-financial이 로드되지 않았습니다.');
    return;
  }

      const ctx = document.getElementById('mixedChart').getContext('2d');
      
      // 하나의 chart, dataset 0=candlestick, 1=scatter(주문), 지표(line)는 SSE시 동적 생성
      mixedChart = new Chart(ctx, {
        data: {
          datasets: [
            {
              label: 'Price',
              type: 'candlestick',
              data: [],
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
              time: { unit: 'minute' }
            },
            yCandles: {
              position: 'left',
              beginAtZero: false
            }
          }
        }
      });

      // SSE
      const evtSource = new EventSource('/sse');
      evtSource.onmessage = function(ev) {
        const parsed = JSON.parse(ev.data);
        switch(parsed.type) {
          case 'candle': {
            // dataset[0] = candlestick
            const dsCandle = mixedChart.data.datasets[0];
            const c = parsed.data; // {x, o, h, l, c}
            let idx = dsCandle.data.findIndex(item => item.x === c.x);
            if(idx >= 0) {
              dsCandle.data[idx] = c;
            } else {
              dsCandle.data.push(c);
            }
            mixedChart.update();
            break;
          }
          case 'indicators': {
            // { time, indicators:[{name, value},...] }
            const iEvt = parsed.data;
            let tVal = iEvt.time;
            iEvt.indicators.forEach(iv => {
              let ds = getOrCreateLineDataset(mixedChart, iv.name);
              // x=tVal, y=iv.value
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
            // dataset[1] = scatter
            const dsOrder = mixedChart.data.datasets[1];
            const od = parsed.data;
            // x=od.time, y=od.price
            let color = (od.side==="buy"||od.side==="bid") ? "green":"red";
            dsOrder.data.push({
              x: od.time,
              y: od.price,
              backgroundColor: color,
              borderColor: color
            });
            mixedChart.update();
            break;
          }
          default:
            console.log("Unknown SSE:", parsed.type, parsed.data);
        }
      };
    });

    // 동적으로 지표(line) dataset 찾거나 생성
    function getOrCreateLineDataset(chart, name) {
      let ds = chart.data.datasets.find(d => d.label === name);
      if(!ds) {
        ds = {
          label: name,
          type: 'line',
          data: [],
          borderColor: pickRandomColor(),
          fill: false,
          pointRadius: 1,
          yAxisID: 'yCandles'  // or separate axis
        };
        chart.data.datasets.push(ds);
      }
      return ds;
    }

    function pickRandomColor() {
      let r = Math.floor(Math.random()*256);
      let g = Math.floor(Math.random()*256);
      let b = Math.floor(Math.random()*256);
      return "rgb(" + r + "," + g + "," + b + ")";
    }
  </script>
</head>
<body>
  <h1>Mixed Chart: Candlestick + All Indicators + Orders</h1>
  <canvas id="mixedChart" width="1200" height="600"></canvas>
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Start : 서버 구동
func (ws *WebServer) Start(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/chart", ws.chartHandler)
	mux.HandleFunc("/sse", ws.sseHandler)

	fmt.Println("[WebServer] Listening on", port)
	return http.ListenAndServe(port, mux)
}
