package visualize

import (
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

var predefinedColorMap = map[string]drawing.Color{
	"2xx": chart.ColorAlternateGreen,
	"4xx": chart.ColorAlternateYellow,
	"5xx": chart.ColorRed,
}

// Visualize draw a line chart and export to a file.
// Currently only supports hourly data for recent 24 hours.
func Visualize(ctx context.Context, title, imgFile string, startTime, endTime time.Time, interval time.Duration, seriesMap *monitoring.TimeSeriesMap, logger *zap.Logger) (int64, error) {
	_, span := trace.GetTracer().Start(ctx, "visualize.Visualize")
	defer span.End()

	span.SetAttributes(
		attribute.String("visualize.title", title),
		attribute.String("visualize.file", imgFile),
		attribute.Int("visualize.series_count", len(*seriesMap)),
	)

	series := []chart.Series{}
	i := 0
	for name, ts := range *seriesMap {
		series = append(series, makeChartTimeSeries(i, name, startTime, endTime, interval, &ts, logger))
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Warn("Failed to close file", zap.Error(err))
		}
	}()
	err = graph.Render(chart.PNG, f)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	stat, err := f.Stat()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}

	span.SetAttributes(attribute.Int64("visualize.file_size", stat.Size()))
	return stat.Size(), nil
}

func makeChartTimeSeries(i int, name string, startTime, endTime time.Time, interval time.Duration, timeSeries *monitoring.TimeSeries, logger *zap.Logger) *chart.TimeSeries {
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
	logger.Debug("Created chart time series", zap.String("name", name), zap.Int("x_values_count", len(cTs.XValues)), zap.Int("y_values_count", len(cTs.YValues)))
	return &cTs
}

func VisualizeSample(ctx context.Context, imgFile string, logger *zap.Logger) error {
	_, span := trace.GetTracer().Start(ctx, "visualize.VisualizeSample")
	defer span.End()
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
	defer func() {
		if err := f.Close(); err != nil {
			logger.Warn("Failed to close file", zap.Error(err))
		}
	}()
	return graph.Render(chart.PNG, f)
}
