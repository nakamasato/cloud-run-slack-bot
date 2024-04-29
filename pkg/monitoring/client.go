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

type Latency struct {
	avg float64
	p50 float64
	p95 float64
	p99 float64
}

type Latencies map[string]Latency

func (l *Latencies) String() string {
	var s []string
	for k, v := range *l {
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

func (mc *Client) GetCloudRunServiceRequestCount(ctx context.Context, service string, aggregationPeriodInSec, duration time.Duration) (*TimeSeriesMap, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_count"},
		},
	}
	endtime := time.Now().UTC() // TODO: set based on aggregationPeriodInSec
	startTime := endtime.Add(-1 * duration).UTC()
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	log.Printf("[%s] get metrics %s (%s -> %s)\n", mc.project, monCon.filter(), startTime, endtime)
	// monitoringpb.Aggregation_ALIGN_SUM
	// monitoringpb.Aggregation_ALIGN_RATE
	// monitoringpb.Aggregation_ALIGN_PERCENTILE_99
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", mc.project),
		Filter: monCon.filter(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: startTime.Unix()},
			EndTime:   &timestamppb.Timestamp{Seconds: endtime.Unix()},
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  &durationpb.Duration{Seconds: int64(aggregationPeriodInSec.Seconds())}, // The value must be at least 60 seconds.
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_SUM,                                     // sum for request count
			GroupByFields:    []string{"resource.revision_name"},
		},
		// PageSize: int32(10000), 100,000 if empty
	}
	return mc.GetRevisionRequestCount(ctx, req)
}

// Not ready
func (mc *Client) GetCloudRunServiceLatency(ctx context.Context, service string, duration time.Duration) (*Latencies, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_latencies"},
		},
	}

	endtime := time.Now().UTC()
	startTime := endtime.Add(-1 * duration).UTC()
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	log.Printf("[%s] get metrics %s (%s -> %s)\n", mc.project, monCon.filter(), startTime, endtime)
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", mc.project),
		Filter: monCon.filter(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: startTime.Unix()},
			EndTime:   &timestamppb.Timestamp{Seconds: endtime.Unix()},
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  &durationpb.Duration{Seconds: 60},            // The value must be at least 60 seconds.
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_PERCENTILE_99, // sum for request count
			GroupByFields:    []string{"resource.revision_name"},
		},
		// PageSize: int32(10000), 100,000 if empty
	}
	return mc.GetRevisionLatency(ctx, req) // TODO: latency should not sum up like request count
}

func (mc *Client) GetRevisionRequestCount(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*TimeSeriesMap, error) {
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
			log.Println("nil")
			continue
		}
		log.Printf("resp %v\n", resp.String())
		revision, ok := resp.Resource.Labels["revision_name"]
		if seriesMap[revision] == nil {
			seriesMap[revision] = TimeSeries{}
		}
		if !ok {
			log.Println("revision_name not found")
			continue
		}

		for i, p := range resp.GetPoints() { // Point per min
			log.Println(p.Value.String())
			log.Printf("Point:%d\tRev:%s\tstart:%s\tend:%s\tvalue:%d\n", i, revision, p.Interval.StartTime.AsTime(), p.Interval.EndTime.AsTime(), p.Value.GetInt64Value())
			val := p.GetValue().GetInt64Value()
			requestCount += val
			cnt[revision] += val
			seriesMap[revision] = append(seriesMap[revision], Point{Time: p.Interval.StartTime.AsTime(), Val: float64(val)})
		}
		loopCnt++
	}
	log.Printf("Request count: %d, %s, %s\n", requestCount, cnt, seriesMap)
	return &seriesMap, nil
}

func (mc *Client) GetRevisionLatency(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*Latencies, error) {

	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	latencies := Latencies{}
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
		revision, ok := resp.Resource.Labels["revision_name"]
		if !ok {
			log.Println("revision_name not found")
			continue
		}

		if _, ok := latencies[revision]; !ok {
			latencies[revision] = Latency{
				avg: 0,
				p50: 0,
				p95: 0,
				p99: 0,
			}
		}

		for i, p := range resp.GetPoints() { // Point per min
			log.Println(p.Value.String())
			log.Printf("Point:%d\tstart:%s\tend:%s\tvalue:%d\n", i, p.Interval.StartTime.AsTime(), p.Interval.EndTime.AsTime(), p.Value.GetInt64Value())
			val := p.GetValue().GetInt64Value()
			requestCount += val
		}
		loopCnt++
	}
	log.Printf("Request count: %d, %s\n", requestCount, latencies.String())
	return &latencies, nil
}

func (mc *Client) Close() error {
	return mc.client.Close()
}
