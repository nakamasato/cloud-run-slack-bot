// https://cloud.google.com/go/docs/reference/cloud.google.com/go/monitoring/latest/apiv3/v2
package monitoring

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
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

type ClientOption func(*Client)

func WithLogger(l *zap.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

func NewMonitoringClient(project string, opts ...ClientOption) (*Client, error) {
	ctx := context.Background()
	client, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("Monitoring client created for project %s\n", project)
	c := &Client{project: project, client: client}

	for _, opt := range opts {
		opt(c)
	}
	// default logger
	if c.logger == nil {
		c.logger = zap.NewExample()
	}
	return c, nil
}

func (mc *Client) GetCloudRunServiceRequestCount(ctx context.Context, service string, aggregationPeriod time.Duration, startTime, endTime time.Time) (*TimeSeriesMap, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_count"},
		},
	}
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	mc.logger.Info("get metrics", zap.String("project", mc.project), zap.String("filter", monCon.filter()), zap.Time("start", startTime), zap.Time("end", endTime))
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
	return mc.aggregateRequestCount(ctx, "response_code_class", "metric", req)
}

// labelType: metric or resource
func (mc *Client) aggregateRequestCount(ctx context.Context, label, labelType string, req *monitoringpb.ListTimeSeriesRequest) (*TimeSeriesMap, error) {
	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	cnt := Counter{}
	seriesMap := TimeSeriesMap{}
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			mc.logger.Info("iterator.Done", zap.Int("loopCnt", loopCnt))
			break
		}
		pageInfo := it.PageInfo()
		mc.logger.Info("page info", zap.String("token", pageInfo.Token), zap.Int("maxSize", pageInfo.MaxSize))
		if err != nil {
			mc.logger.Error("error", zap.Error(err))
			return nil, err
		}
		if resp == nil {
			continue
		}
		mc.logger.Info("resp", zap.String("resp", resp.String()))
		var labelValue string
		var ok bool
		switch labelType {
		case "metric":
			labelValue, ok = resp.Metric.Labels[label]
		case "resource":
			labelValue, ok = resp.Resource.Labels[label]
		default:
			mc.logger.Error("invalid label type", zap.String("labelType", labelType))
			return nil, err
		}
		if seriesMap[labelValue] == nil {
			seriesMap[labelValue] = TimeSeries{}
		}
		if !ok {
			mc.logger.Error("Metric label not found", zap.String("label", label))
			continue
		}

		for i, p := range resp.GetPoints() { // Point per min
			mc.logger.Info("Point", zap.Int("i", i), zap.String("label", label), zap.String("labelValue", labelValue), zap.Time("start", p.Interval.StartTime.AsTime()), zap.Time("end", p.Interval.EndTime.AsTime()), zap.Int64("value", p.Value.GetInt64Value()))
			val := p.GetValue().GetInt64Value()
			requestCount += val
			cnt[labelValue] += val
			seriesMap[labelValue] = append(seriesMap[labelValue], Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	mc.logger.Info("Request count", zap.Int64("requestCount", requestCount), zap.Any("counter", cnt), zap.Any("seriesMap", seriesMap))
	return &seriesMap, nil
}

func (mc *Client) aggregateRequestLatency(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*TimeSeries, error) {
	it := mc.client.ListTimeSeries(ctx, req)
	var loopCnt int
	cnt := Counter{}
	series := TimeSeries{}
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			mc.logger.Info("iterator.Done", zap.Int("loopCnt", loopCnt))
			break
		}
		pageInfo := it.PageInfo()
		mc.logger.Info("page info", zap.String("token", pageInfo.Token), zap.Int("maxSize", pageInfo.MaxSize))
		if err != nil {
			mc.logger.Error("failed to get page info", zap.Error(err))
			return nil, err
		}
		if resp == nil {
			mc.logger.Info("page info resp is nil")
			continue
		}
		mc.logger.Info("successfully got page info", zap.String("resp", resp.String()))

		for i, p := range resp.GetPoints() { // Point per min
			log.Println(p.Value.String())
			mc.logger.Info("Latency Point", zap.Int("i", i), zap.Time("start", p.Interval.StartTime.AsTime()), zap.Time("end", p.Interval.EndTime.AsTime()), zap.Int64("value", p.Value.GetInt64Value()))
			val := p.GetValue().GetDoubleValue()
			series = append(series, Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	mc.logger.Info("Request Latency", zap.Any("counter", cnt), zap.Any("series", series), zap.Int("loopCnt", loopCnt))
	return &series, nil
}

func (mc *Client) GetCloudRunServiceRequestLatencies(ctx context.Context, service string, aggregationPeriod time.Duration, startTime, endTime time.Time) (*TimeSeriesMap, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_latencies"},
		},
	}
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	mc.logger.Info("get metrics", zap.String("project", mc.project), zap.String("filter", monCon.filter()), zap.Time("start", startTime), zap.Time("end", endTime))
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
		series, err := mc.aggregateRequestLatency(ctx, req)
		if err != nil {
			mc.logger.Error("failed to get request latency", zap.Error(err))
			return nil, err
		}
		timeSeriesMap[aligner.String()] = *series
	}
	return &timeSeriesMap, nil
}

func (mc *Client) Close() error {
	return mc.client.Close()
}
