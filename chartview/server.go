// chartview/server.go
package chartview

import (
	"log"
	"net/http"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// StartChartServer : 간단한 http.Server 예시
func StartChartServer(addr string) {
	// 메인 라우트
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
            <h2>Raccoon Bot Chart</h2>
            <p><a href="/chart">Go To Candle Chart</a></p>
            </body></html>`))
	})

	// /chart 라우트
	http.HandleFunc("/chart", candleChartHandler)

	log.Printf("[ChartView] listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("[ChartView] server error: %v\n", err)
	}
}

func candleChartHandler(w http.ResponseWriter, r *http.Request) {
	page := components.NewPage()
	page.PageTitle = "Raccoon Bot Chart"

	// 1) 봉차트
	kline := buildCandleChart()
	// 2) MACD 라인
	macdLine := buildMACDChart()
	// 3) RSI 라인
	rsiLine := buildRSIChart()

	page.AddCharts(kline, macdLine, rsiLine)
	_ = page.Render(w)
}

// buildCandleChart : 봉차트(Kline) + 원한다면 볼린저밴드/MA도 Overlap 가능
func buildCandleChart() *charts.Kline {
	// (1) 실시간 봉 가져오기
	candles := GlobalChartData.GetCandles()
	n := len(candles)
	if n == 0 {
		return charts.NewKLine() // 데이터 없으면 빈
	}

	xVals := make([]string, n)
	kValues := make([]opts.KlineData, n)

	// go-echarts Kline은 [open, close, low, high] 순서가 표준
	for i, c := range candles {
		xVals[i] = c.Time.Format("01/02 15:04")
		kValues[i] = opts.KlineData{
			Value: [4]float64{c.Open, c.Close, c.Low, c.High},
		}
	}

	kline := charts.NewKLine()
	kline.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Candle Chart",
			Show:  opts.Bool(true), // bool -> types.Bool
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show: opts.Bool(true),
		}),
		charts.WithLegendOpts(opts.Legend{
			Show: opts.Bool(true),
		}),
		charts.WithYAxisOpts(opts.YAxis{
			// ...
		}),
	)
	// Series 설정
	kline.SetXAxis(xVals).
		AddSeries("KLine", kValues).
		SetSeriesOptions(charts.WithItemStyleOpts(opts.ItemStyle{
			Color:        "#ec0000", // 음봉 내부색
			Color0:       "#00da3c", // 양봉 내부색
			BorderColor:  "#8A0000",
			BorderColor0: "#008F28",
		}))
	return kline
}

// buildMACDChart : MACD, Signal, Hist 라인을 한 차트에 겹침
func buildMACDChart() *charts.Line {
	// (1) 지표 가져오기
	_, macdArr, macdSigArr, macdHistArr := GlobalChartData.GetIndicators()
	if len(macdArr) == 0 {
		return charts.NewLine()
	}

	xVals := GlobalChartData.GetTimeAxis()
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "MACD",
			Show:  opts.Bool(true),
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show: opts.Bool(true),
		}),
		charts.WithLegendOpts(opts.Legend{
			Show: opts.Bool(true),
		}),
	)

	macdSeries := make([]opts.LineData, len(macdArr))
	sigSeries := make([]opts.LineData, len(macdSigArr))
	histSeries := make([]opts.LineData, len(macdHistArr))

	for i := 0; i < len(macdArr); i++ {
		macdSeries[i] = opts.LineData{Value: macdArr[i]}
		sigSeries[i] = opts.LineData{Value: macdSigArr[i]}
		histSeries[i] = opts.LineData{Value: macdHistArr[i]}
	}

	line.SetXAxis(xVals).
		AddSeries("MACD", macdSeries).
		AddSeries("Signal", sigSeries).
		AddSeries("Hist", histSeries).
		SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{
			Smooth: opts.Bool(true),
		}))
	return line
}

// buildRSIChart : RSI
func buildRSIChart() *charts.Line {
	rsiArr, _, _, _ := GlobalChartData.GetIndicators()
	if len(rsiArr) == 0 {
		return charts.NewLine()
	}

	xVals := GlobalChartData.GetTimeAxis()
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "RSI(14)",
			Show:  opts.Bool(true),
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show: opts.Bool(true),
		}),
		charts.WithLegendOpts(opts.Legend{
			Show: opts.Bool(true),
		}),
	)

	rsiSeries := make([]opts.LineData, len(rsiArr))
	for i, v := range rsiArr {
		rsiSeries[i] = opts.LineData{Value: v}
	}

	line.SetXAxis(xVals).
		AddSeries("RSI(14)", rsiSeries).
		SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(true)}))

	return line
}
