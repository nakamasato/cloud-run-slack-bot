package visualize

import (
	"os"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/wcharczuk/go-chart/v2"
)

// Visualize draw a line chart and export to a file.
// Currently only supports hourly data for recent 24 hours.
func Visualize(imgFile string, startTime, endTime time.Time, seriesMap *monitoring.TimeSeriesMap) (int64, error) {

	series := []chart.Series{}
	i := 0
	for name, ts := range *seriesMap {
		series = append(series, makeChartTimeSeries(name, startTime, endTime, time.Hour, &ts))
		i++
	}
	graph := chart.Chart{
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

func makeChartTimeSeries(name string, startTime, endTime time.Time, interval time.Duration, timeSeries *monitoring.TimeSeries) *chart.TimeSeries {
	cTs := chart.TimeSeries{
		Name: name,
	}
	counter := map[time.Time]float64{}
	for _, p := range *timeSeries {
		counter[p.Time] += p.Val
	}
	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		cTs.XValues = append(cTs.XValues, t)
		cTs.YValues = append(cTs.YValues, counter[t])
	}
	return &cTs
}

func VisualizeSample(imgFile string) error {
	graph := chart.Chart{
		Series: []chart.Series{
			chart.ContinuousSeries{
				XValues: []float64{1.0, 2.0, 3.0, 4.0},
				YValues: []float64{1.0, 2.0, 3.0, 4.0},
			},
		},
	}

	f, err := os.Create(imgFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return graph.Render(chart.PNG, f)
}
