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
}

func NewMonitoringClient(project string) (*Client, error) {
	ctx := context.Background()
	client, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("Monitoring client created for project %s\n", project)
	return &Client{project: project, client: client}, nil
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
	log.Printf("[%s] get metrics %s (%s -> %s)\n", mc.project, monCon.filter(), startTime, endTime)
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
			log.Printf("iterator.Done %d\n", loopCnt)
			break
		}
		pageInfo := it.PageInfo()
		log.Printf("[page info] token:%s\tMaxSize:%d\n", pageInfo.Token, pageInfo.MaxSize)
		if err != nil {
			log.Printf("err %v\n", err)
			return nil, err
		}
		if resp == nil {
			continue
		}
		log.Printf("resp %v\n", resp.String())
		var labelValue string
		var ok bool
		switch labelType {
		case "metric":
			labelValue, ok = resp.Metric.Labels[label]
		case "resource":
			labelValue, ok = resp.Resource.Labels[label]
		default:
			log.Printf("Invalid label type %s\n", labelType)
			return nil, fmt.Errorf("invalid label type %s", labelType)
		}
		if seriesMap[labelValue] == nil {
			seriesMap[labelValue] = TimeSeries{}
		}
		if !ok {
			log.Printf("Metric label '%s' not found", label)
			continue
		}

		for i, p := range resp.GetPoints() { // Point per min
			log.Println(p.Value.String())
			log.Printf("Point:%d\t%s:%s\tstart:%s\tend:%s\tvalue:%d\n", i, label, labelValue, p.Interval.StartTime.AsTime(), p.Interval.EndTime.AsTime(), p.Value.GetInt64Value())
			val := p.GetValue().GetInt64Value()
			requestCount += val
			cnt[labelValue] += val
			seriesMap[labelValue] = append(seriesMap[labelValue], Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	log.Printf("Request count:%d\nCounter:\n%s\nseriesMap:\n%s\n", requestCount, cnt, seriesMap)
	return &seriesMap, nil
}

func (mc *Client) aggregateRequestLatency(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*TimeSeries, error) {
	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	cnt := Counter{}
	series := TimeSeries{}
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			log.Printf("iterator.Done %d\n", loopCnt)
			break
		}
		pageInfo := it.PageInfo()
		log.Printf("[page info] token:%s\tMaxSize:%d\n", pageInfo.Token, pageInfo.MaxSize)
		if err != nil {
			log.Printf("err %v\n", err)
			return nil, err
		}
		if resp == nil {
			log.Println("nil")
			continue
		}
		log.Printf("resp %v\n", resp.String())

		for i, p := range resp.GetPoints() { // Point per min
			log.Println(p.Value.String())
			log.Printf("Latency Point:%d\tstart:%s\tend:%s\tvalue:%d\n", i, p.Interval.StartTime.AsTime(), p.Interval.EndTime.AsTime(), p.Value.GetInt64Value())
			val := p.GetValue().GetDoubleValue()
			series = append(series, Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	log.Printf("Request count:%d\nCounter:\n%s\nseries:\n%s\n", requestCount, cnt, series)
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
	log.Printf("[%s] get metrics %s (%s -> %s)\n", mc.project, monCon.filter(), startTime, endTime)
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
			return nil, err
		}
		timeSeriesMap[aligner.String()] = *series
	}
	return &timeSeriesMap, nil
}

func (mc *Client) Close() error {
	return mc.client.Close()
}
