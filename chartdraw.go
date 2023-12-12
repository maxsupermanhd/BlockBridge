package main

import (
	"bytes"
	"io"
	"log"
	"time"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

func chartGenGridLines(begin, end, step int, style chart.Style) []chart.GridLine {
	g := []chart.GridLine{}
	for i := begin; i <= end; i += step {
		g = append(g, chart.GridLine{Value: float64(i), Style: style})
	}
	return g
}

func drawTPS(keys []time.Time, tpsValues []float64, playercountValues []float64) io.Reader {

	TPSseries := chart.TimeSeries{
		XValues: keys,
		YValues: tpsValues,
		Name:    "Raw TPS",
	}

	playercountSeries := chart.TimeSeries{
		XValues: keys,
		YValues: playercountValues,
		Name:    "Player count",
		Style:   chart.Style{StrokeColor: drawing.Color{R: 128, G: 16, B: 16, A: 255}},
		YAxis:   chart.YAxisSecondary,
	}

	AvgTPSseries := chart.SMASeries{
		InnerSeries: TPSseries,
		Period:      20 * 60,
		Name:        "Average TPS",
		Style:       chart.Style{StrokeWidth: 5},
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			ValueFormatter: func(v interface{}) string {
				if typed, isTyped := v.(float64); isTyped {
					return time.Unix(0, int64(typed)).Format("02 Jan 15:04")
				}
				return ""
			},
		},
		YAxis: chart.YAxis{
			ValueFormatter: chart.IntValueFormatter,
			Name:           "TPS",
			GridLines:      chartGenGridLines(1, 20, 1, chart.Style{StrokeWidth: 2, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}),
			Range:          &chart.ContinuousRange{Min: 0, Max: 20},
		},
		YAxisSecondary: chart.YAxis{
			ValueFormatter: chart.IntValueFormatter,
			Name:           "Player count",
			GridLines:      chartGenGridLines(10, 200, 10, chart.Style{StrokeColor: drawing.ColorTransparent}),
			Range:          &chart.ContinuousRange{Min: 0, Max: 200},
		},
		Series: []chart.Series{
			TPSseries,
			AvgTPSseries,
			playercountSeries,
		},
		Background: chart.Style{Padding: chart.Box{Left: 20, Top: 40}},
		Height:     400,
		Width:      1100,
		// Title:      fmt.Sprintf("Constantiam %s - %s", keys[0].Format("02 Jan 15:04"), keys[len(keys)-1].Format("02 Jan 15:04")),
		// TitleStyle: chart.Style{Padding: chart.Box{Bottom: 150}},
	}
	graph.Elements = []chart.Renderable{
		chart.LegendThin(&graph),
	}
	buf := bytes.NewBufferString("")
	err := graph.Render(chart.PNG, buf)
	if err != nil {
		log.Println(err)
	}
	return buf
}

func drawAvgPlayercount(keys []time.Time, avgplayercount []float64) io.Reader {
	playercountSeries := chart.TimeSeries{
		XValues: keys,
		YValues: avgplayercount,
		Name:    "Average player count",
		Style:   chart.Style{StrokeColor: drawing.Color{R: 128, G: 16, B: 16, A: 255}},
		YAxis:   chart.YAxisPrimary,
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			ValueFormatter: func(v interface{}) string {
				if typed, isTyped := v.(float64); isTyped {
					return time.Unix(0, int64(typed)).Format("02 Jan 15:04")
				}
				return ""
			},
		},
		YAxis: chart.YAxis{
			ValueFormatter: chart.IntValueFormatter,
			Name:           "Average player count",
			GridLines:      chartGenGridLines(10, 200, 10, chart.Style{StrokeWidth: 2, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}),
			Range:          &chart.ContinuousRange{Min: 0, Max: 200},
		},
		Series: []chart.Series{
			playercountSeries,
		},
		Background: chart.Style{Padding: chart.Box{Left: 20, Top: 40}},
		Height:     400,
		Width:      1100,
		Title:      "Constantiam player count",
	}
	buf := bytes.NewBufferString("")
	err := graph.Render(chart.PNG, buf)
	if err != nil {
		log.Println(err)
	}
	return buf
}
