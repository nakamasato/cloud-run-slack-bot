package visualize

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/wcharczuk/go-chart/v2"
)

// Visualize draw a line chart and export to a file.
// Currently only supports hourly data for recent 24 hours.
func Visualize(imgFile string, seriesMap *monitoring.TimeSeriesMap) (int64, error) {
	xaxis := []float64{}
	yaxis := []float64{}
	series := []chart.Series{}
	now := time.Now()
	i := 0
	for name, ts := range *seriesMap {
		hour2val := map[int]float64{}
		for _, p := range ts {
			hour2val[p.Time.Hour()] += p.Val
		}
		for j := 23; j >=0; j-- {
			t := now.Add(-time.Duration(j) * time.Hour)
			log.Printf("name: %s, xaxis: %d, yaxis: %d\n", name, t.Hour(), hour2val[t.Hour()])
			xaxis = append(xaxis, float64(t.Hour()))
			yaxis = append(yaxis, hour2val[t.Hour()])
		}
		series = append(series, chart.ContinuousSeries{
			Name: name,
			Style: chart.Style{
				StrokeColor: chart.GetDefaultColor(i).WithAlpha(64),
			},
			XValues: xaxis,
			YValues: yaxis,
		})
		i++
	}
	ticks := []chart.Tick{}
	for i := 23; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Hour)
		ticks = append(ticks, chart.Tick{Value: float64(t.Hour()), Label: fmt.Sprintf("%d:00", t.Hour())})
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name:  "Time",
			Ticks: ticks,
		},
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
