package main

import (
	"bytes"
	"io"
	"time"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

func drawTPS(keys []time.Time, values []float64) io.Reader {

	graph := chart.Chart{
		XAxis: chart.XAxis{
			ValueFormatter: chart.TimeHourValueFormatter,
			Name:           "Time",
			// ValueFormatter: chart.TimeValueFormatterWithFormat("02 Jan 06 15:04"),
		},
		YAxis: chart.YAxis{
			ValueFormatter: chart.IntValueFormatter,
			Name:           "TPS",
			GridLines: []chart.GridLine{
				{Value: 1, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 2, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 3, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 4, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 5, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 6, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 7, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 8, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 9, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 10, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 11, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 12, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 13, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 14, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 15, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 16, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 17, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 18, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 19, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
				{Value: 20, Style: chart.Style{StrokeWidth: 2, Hidden: false, StrokeColor: drawing.Color{R: 40, G: 40, B: 40, A: 60}}},
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: keys,
				YValues: values,
			},
		},
		Background: chart.Style{Padding: chart.Box{Top: 50}},
		Title:      "Constantiam TPS for around past 24h",
		Height:     500,
	}
	buf := bytes.NewBufferString("")
	must(graph.Render(chart.PNG, buf))
	return buf
}
