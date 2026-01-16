// https://cloud.google.com/go/docs/reference/cloud.google.com/go/monitoring/latest/apiv3/v2
package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MonitorFilter map[string]string

type MonitorCondition struct {
	Project string // used for name in ListTimeSeriesRequest
	Filters []MonitorFilter
}

type Counter map[string]int64

func (c Counter) String() string {
	var s []string
	for k, v := range c {
		s = append(s, fmt.Sprintf("- %s:%d", k, v))
	}
	return strings.Join(s, "\n")
}

type Point struct {
	Time time.Time
	Val  float64
}
type TimeSeries []Point

func (ts TimeSeries) String() string {
	var s []string
	for _, v := range ts {
		s = append(s, fmt.Sprintf("%v", v.Val))
	}
	return strings.Join(s, ",")
}

type TimeSeriesMap map[string]TimeSeries

func (ts TimeSeriesMap) String() string {
	var s []string
	for k, v := range ts {
		s = append(s, fmt.Sprintf("%s:%v", k, v))
	}
	return strings.Join(s, "\n")
}

func (c *MonitorCondition) filter() string {
	var filters []string
	for _, f := range c.Filters {
		for k, v := range f {
			filters = append(filters, fmt.Sprintf("%s = \"%s\"", k, v))
		}
	}
	return strings.Join(filters, " AND\n ")
}

type Client struct {
	project string
	client  *monitoring.MetricClient
	logger  *zap.Logger
}

func NewMonitoringClient(project string, logger *zap.Logger) (*Client, error) {
	ctx := context.Background()
	client, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, err
	}
	logger.Info("Monitoring client created", zap.String("project", project))
	return &Client{project: project, client: client, logger: logger}, nil
}

func (mc *Client) GetCloudRunServiceRequestCount(ctx context.Context, service string, aggregationPeriod time.Duration, startTime, endTime time.Time) (*TimeSeriesMap, error) {
	ctx, span := trace.GetTracer().Start(ctx, "monitoring.GetCloudRunServiceRequestCount")
	defer span.End()

	span.SetAttributes(
		attribute.String("monitoring.project", mc.project),
		attribute.String("monitoring.service", service),
		attribute.String("monitoring.aggregation_period", aggregationPeriod.String()),
	)

	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_count"},
		},
	}
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	mc.logger.Info("Getting metrics",
		zap.String("project", mc.project),
		zap.String("filter", monCon.filter()),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))
	// monitoringpb.Aggregation_ALIGN_SUM
	// monitoringpb.Aggregation_ALIGN_RATE
	// monitoringpb.Aggregation_ALIGN_PERCENTILE_99
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", mc.project),
		Filter: monCon.filter(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: startTime.Unix()},
			EndTime:   &timestamppb.Timestamp{Seconds: endTime.Unix()},
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  &durationpb.Duration{Seconds: int64(aggregationPeriod.Seconds())}, // The value must be at least 60 seconds.
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_SUM,                                // sum for request count
			GroupByFields:    []string{"resource.revision_name"},
		},
		// PageSize: int32(10000), 100,000 if empty
	}
	result, err := mc.GetRequestCountByLabel(ctx, "response_code_class", "metric", req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

// labelType: metric or resource
func (mc *Client) GetRequestCountByLabel(ctx context.Context, label, labelType string, req *monitoringpb.ListTimeSeriesRequest) (*TimeSeriesMap, error) {
	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	cnt := Counter{}
	seriesMap := TimeSeriesMap{}
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			mc.logger.Debug("Iterator done", zap.Int("loop_count", loopCnt))
			break
		}
		pageInfo := it.PageInfo()
		mc.logger.Debug("Page info", zap.String("token", pageInfo.Token), zap.Int("max_size", pageInfo.MaxSize))
		if err != nil {
			mc.logger.Error("Error iterating time series", zap.Error(err))
			return nil, err
		}
		if resp == nil {
			mc.logger.Debug("Nil response")
			continue
		}
		mc.logger.Debug("Response", zap.String("response", resp.String()))
		var labelValue string
		var ok bool
		switch labelType {
		case "metric":
			labelValue, ok = resp.Metric.Labels[label]
		case "resource":
			labelValue, ok = resp.Resource.Labels[label]
		default:
			mc.logger.Error("Invalid label type", zap.String("label_type", labelType))
			return nil, fmt.Errorf("invalid label type %s", labelType)
		}
		if seriesMap[labelValue] == nil {
			seriesMap[labelValue] = TimeSeries{}
		}
		if !ok {
			mc.logger.Warn("Metric label not found", zap.String("label", label))
			continue
		}

		for i, p := range resp.GetPoints() { // Point per min
			mc.logger.Debug("Point value", zap.String("value", p.Value.String()))
			mc.logger.Debug("Point details",
				zap.Int("index", i),
				zap.String("label", label),
				zap.String("label_value", labelValue),
				zap.Time("start_time", p.Interval.StartTime.AsTime()),
				zap.Time("end_time", p.Interval.EndTime.AsTime()),
				zap.Int64("value", p.Value.GetInt64Value()))
			val := p.GetValue().GetInt64Value()
			requestCount += val
			cnt[labelValue] += val
			seriesMap[labelValue] = append(seriesMap[labelValue], Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	mc.logger.Debug("Request count summary",
		zap.Int64("request_count", requestCount),
		zap.String("counter", cnt.String()),
		zap.String("series_map", seriesMap.String()))
	return &seriesMap, nil
}

func (mc *Client) AggregateLatencies(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*TimeSeries, error) {
	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	cnt := Counter{}
	series := TimeSeries{}
	labelValue := "p99"
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			mc.logger.Debug("Iterator done", zap.Int("loop_count", loopCnt))
			break
		}
		pageInfo := it.PageInfo()
		mc.logger.Debug("Page info", zap.String("token", pageInfo.Token), zap.Int("max_size", pageInfo.MaxSize))
		if err != nil {
			mc.logger.Error("Error iterating time series", zap.Error(err))
			return nil, err
		}
		if resp == nil {
			mc.logger.Debug("Nil response")
			continue
		}
		mc.logger.Debug("Response", zap.String("response", resp.String()))

		for i, p := range resp.GetPoints() { // Point per min
			mc.logger.Debug("Latency point value", zap.String("value", p.Value.String()))
			mc.logger.Debug("Latency point details",
				zap.Int("index", i),
				zap.String("label", "revision_name"),
				zap.String("label_value", labelValue),
				zap.Time("start_time", p.Interval.StartTime.AsTime()),
				zap.Time("end_time", p.Interval.EndTime.AsTime()),
				zap.Int64("value", p.Value.GetInt64Value()))
			val := p.GetValue().GetDoubleValue()
			series = append(series, Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	mc.logger.Debug("Latency summary",
		zap.Int64("request_count", requestCount),
		zap.String("counter", cnt.String()),
		zap.String("series", series.String()))
	return &series, nil
}

func (mc *Client) GetCloudRunServiceRequestLatencies(ctx context.Context, service string, aggregationPeriod time.Duration, startTime, endTime time.Time) (*TimeSeriesMap, error) {
	ctx, span := trace.GetTracer().Start(ctx, "monitoring.GetCloudRunServiceRequestLatencies")
	defer span.End()

	span.SetAttributes(
		attribute.String("monitoring.project", mc.project),
		attribute.String("monitoring.service", service),
		attribute.String("monitoring.aggregation_period", aggregationPeriod.String()),
	)

	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_latencies"},
		},
	}
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	mc.logger.Info("Getting metrics",
		zap.String("project", mc.project),
		zap.String("filter", monCon.filter()),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))
	aligners := []monitoringpb.Aggregation_Aligner{
		monitoringpb.Aggregation_ALIGN_PERCENTILE_50,
		monitoringpb.Aggregation_ALIGN_PERCENTILE_95,
		monitoringpb.Aggregation_ALIGN_PERCENTILE_99,
	}
	timeSeriesMap := TimeSeriesMap{}

	for _, aligner := range aligners {
		req := &monitoringpb.ListTimeSeriesRequest{
			Name:   fmt.Sprintf("projects/%s", mc.project),
			Filter: monCon.filter(),
			Interval: &monitoringpb.TimeInterval{
				StartTime: &timestamppb.Timestamp{Seconds: startTime.Unix()},
				EndTime:   &timestamppb.Timestamp{Seconds: endTime.Unix()},
			},
			Aggregation: &monitoringpb.Aggregation{
				AlignmentPeriod:  &durationpb.Duration{Seconds: int64(aggregationPeriod.Seconds())}, // The value must be at least 60 seconds.
				PerSeriesAligner: aligner,
			},
			// PageSize: int32(10000), 100,000 if empty
		}
		series, err := mc.AggregateLatencies(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		timeSeriesMap[aligner.String()] = *series
	}
	return &timeSeriesMap, nil
}

func (mc *Client) Close() error {
	return mc.client.Close()
}
