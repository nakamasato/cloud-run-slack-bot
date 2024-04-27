package visualize

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/snapshot-chromedp/render"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
)

// generate random data
func generateRandomData() monitoring.TimeSeries {
	items := monitoring.TimeSeries{}
	for i := 0; i < 7; i++ {
		items = append(items, int64(rand.Intn(300))) // opts.LineData{Value: rand.Intn(300)}
	}
	return items
}

var (
	itemCnt = 7
	weeks   = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
)

func drawLineChart(title, subtitle string, xAxis *[]string, data *monitoring.TimeSeriesMap) *charts.Line {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			BackgroundColor: "#FFFFFF",
		}),
		// Don't forget disable the Animation
		charts.WithAnimation(false),
		charts.WithTitleOpts(opts.Title{
			Title:    title,
			Subtitle: subtitle,
			Right:    "40%",
		}),
		charts.WithLegendOpts(opts.Legend{Right: "80%"}),
	)
	if xAxis != nil {
		line.SetXAxis(xAxis)
	}
	for name, items := range *data {
		data := getLineData(&items)
		log.Println(fmt.Sprintf("name: %s, data: %v", name, data))
		line.AddSeries(name, *data)
	}
	return line
}

// Visualize generates a random line chart
func Visualize(title, subtitle, fileName string, xAxis *[]string, data *monitoring.TimeSeriesMap) {
	// data := monitoring.TimeSeriesMap{}
	// data["Category A"] = generateRandomData()
	// data["Category B"] = generateRandomData()
	line := drawLineChart(title, subtitle, xAxis, data)
	render.MakeChartSnapshot(line.RenderContent(), fileName)
}

func getLineData(data *monitoring.TimeSeries) *[]opts.LineData {
	items := make([]opts.LineData, 0)
	for i, v := range *data {
		items = append(items, opts.LineData{Value: v, XAxisIndex: i})
	}
	return &items
}