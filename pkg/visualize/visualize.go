package visualize

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var predefinedColorMap = map[string]drawing.Color{
	"2xx": chart.ColorAlternateGreen,
	"4xx": chart.ColorAlternateYellow,
	"5xx": chart.ColorRed,
}

// Visualize draw a line chart and export to a file.
// Currently only supports hourly data for recent 24 hours.
func Visualize(title, imgFile string, startTime, endTime time.Time, interval time.Duration, seriesMap *monitoring.TimeSeriesMap) (int64, error) {

	series := []chart.Series{}
	i := 0
	for name, ts := range *seriesMap {
		series = append(series, makeChartTimeSeries(i, name, startTime, endTime, interval, &ts))
		i++
	}
	// ticks := []chart.Tick{}
	// for t := startTime.Truncate(time.Hour); t.Before(endTime); t = t.Add(time.Hour) {
	// 	ticks = append(ticks, chart.Tick{Value: float64(t.Unix()), Label: t.Format("15")})
	// }
	graph := chart.Chart{
		Title: title,
		// XAxis: chart.XAxis{
		// 	Name:  "Time",
		// 	Ticks: ticks,
		// },
		Series: series,
	}

	f, err := os.Create(imgFile)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	err = graph.Render(chart.PNG, f)
	if err != nil {
		return 0, err
	}
	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func makeChartTimeSeries(i int, name string, startTime, endTime time.Time, interval time.Duration, timeSeries *monitoring.TimeSeries) *chart.TimeSeries {
	color, ok := predefinedColorMap[name]
	if !ok {
		color = chart.GetDefaultColor(i)
	}
	cTs := chart.TimeSeries{
		Name: name,
		Style: chart.Style{
			StrokeColor: color.WithAlpha(64),
			FillColor:   color.WithAlpha(64),
		},
	}
	counter := map[time.Time]float64{}
	for _, p := range *timeSeries {
		counter[p.Time] += p.Val
	}
	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		cTs.XValues = append(cTs.XValues, t)
		cTs.YValues = append(cTs.YValues, counter[t])
	}
	log.Printf("name: %s\nXValues(%d): %v\nYValues(%d): %v", name, len(cTs.XValues), cTs.XValues, len(cTs.YValues), cTs.YValues)
	return &cTs
}

func VisualizeSample(imgFile string) error {
	durationInMin := 24 * 60
	intervalInMin := 5

	tsSuccess := chart.TimeSeries{
		Name: "Success",
		Style: chart.Style{
			StrokeColor: chart.ColorAlternateGreen.WithAlpha(64),
			FillColor:   chart.ColorAlternateGreen.WithAlpha(64),
		},
	}
	tsError := chart.TimeSeries{
		Name: "Error",
		Style: chart.Style{
			StrokeColor: chart.ColorRed.WithAlpha(64),
			FillColor:   chart.ColorRed.WithAlpha(64),
		},
	}
	for i := 0; i < durationInMin/intervalInMin; i++ {
		tsSuccess.XValues = append(tsSuccess.XValues, time.Now().Add(time.Minute*time.Duration(intervalInMin*-i)))
		tsSuccess.YValues = append(tsSuccess.YValues, rand.Float64()*100)
		tsError.XValues = append(tsError.XValues, time.Now().Add(time.Minute*time.Duration(intervalInMin*-i)))
		tsError.YValues = append(tsError.YValues, rand.Float64()*10)
	}

	graph := chart.Chart{
		Title: "Sample Chart",
		Series: []chart.Series{
			tsSuccess, tsError,
		},
	}

	f, err := os.Create(imgFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return graph.Render(chart.PNG, f)
}
