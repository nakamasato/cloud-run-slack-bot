package visualize

import (
	"fmt"
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
	for name, ts := range *seriesMap {
		for _, p := range ts {
			xaxis = append(xaxis, float64(p.Time.Hour()))
			yaxis = append(yaxis, p.Val)
		}
		series = append(series, chart.ContinuousSeries{
			Name:    name,
			XValues: xaxis,
			YValues: yaxis,
		})
	}
	now := time.Now()
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
