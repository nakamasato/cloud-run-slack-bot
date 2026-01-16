package visualize

import (
	"reflect"
	"testing"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/wcharczuk/go-chart/v2"
	"go.uber.org/zap"
)

func TestMakeChartTimeSeries(t *testing.T) {
	tests := []struct {
		name       string
		startTime  time.Time
		endTime    time.Time
		interval   time.Duration
		timeSeries *monitoring.TimeSeries
		want       *chart.TimeSeries
	}{
		{
			name:      "test",
			startTime: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2021, 1, 1, 1, 0, 0, 0, time.UTC),
			interval:  10 * time.Minute,
			timeSeries: &monitoring.TimeSeries{
				{Time: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), Val: 1},
				{Time: time.Date(2021, 1, 1, 0, 10, 0, 0, time.UTC), Val: 2},
			},
			want: &chart.TimeSeries{
				Name: "test",
				Style: chart.Style{
					StrokeColor: chart.GetDefaultColor(0).WithAlpha(64),
					FillColor:   chart.GetDefaultColor(0).WithAlpha(64),
				},
				XValues: []time.Time{
					time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2021, 1, 1, 0, 10, 0, 0, time.UTC),
					time.Date(2021, 1, 1, 0, 20, 0, 0, time.UTC),
					time.Date(2021, 1, 1, 0, 30, 0, 0, time.UTC),
					time.Date(2021, 1, 1, 0, 40, 0, 0, time.UTC),
					time.Date(2021, 1, 1, 0, 50, 0, 0, time.UTC),
				},
				YValues: []float64{1, 2, 0, 0, 0, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop() // Use no-op logger for tests
			if got := makeChartTimeSeries(0, tt.name, tt.startTime, tt.endTime, tt.interval, tt.timeSeries, logger); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("XValues = %v, want %v", got, tt.want)
			}
		})
	}
}
