package visualize

import (
	"math/rand"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/snapshot-chromedp/render"
)

// generate random data for bar chart
func generateBarItems() []opts.LineData {
	items := make([]opts.LineData, 0)
	for i := 0; i < 7; i++ {
		items = append(items, opts.LineData{Value: rand.Intn(300)})
	}
	return items
}

var (
	itemCnt = 7
	weeks   = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
)

func lineChart() *charts.Line {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			BackgroundColor: "#FFFFFF",
		}),
		// Don't forget disable the Animation
		charts.WithAnimation(false),
		charts.WithTitleOpts(opts.Title{
			Title:    "title and legend options",
			Subtitle: "go-echarts is an awesome chart library written in Golang",
			Link:     "https://github.com/go-echarts/go-echarts",
			Right:    "40%",
		}),
		charts.WithLegendOpts(opts.Legend{Right: "80%"}),
	)
	line.SetXAxis(weeks).
		AddSeries("Category A", generateBarItems()).
		AddSeries("Category B", generateBarItems())
	return line
}

func Visualize(name string) {
	render.MakeChartSnapshot(lineChart().RenderContent(), name)
}
