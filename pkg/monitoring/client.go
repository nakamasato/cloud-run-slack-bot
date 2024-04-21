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
	config  *MonitorCondition
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

func (mc *Client) GetCloudRunServiceRequestCount(ctx context.Context, service string, duration time.Duration) (*Counter, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_count"},
		},
	}
	cnt, err := mc.GetRevisionRequestCount(ctx, monCon, duration)
	if err != nil {
		return nil, err
	}
	return cnt, nil
}

// Not ready
func (mc *Client) GetCloudRunServiceLatency(ctx context.Context, service string, duration time.Duration) (*Counter, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_latencies"},
		},
	}
	return mc.GetRevisionRequestCount(ctx, monCon, duration) // TODO: latency should not sum up like request count
}

func (mc *Client) GetRevisionRequestCount(ctx context.Context, monCon MonitorCondition, duration time.Duration) (*Counter, error) {
	endtime := time.Now().UTC()
	startTime := endtime.Add(-1 * duration).UTC()
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	log.Printf("[%s] get metrics %s (%s -> %s)\n", monCon.Project, monCon.filter(), startTime, endtime)
	// monitoringpb.Aggregation_ALIGN_SUM
	// monitoringpb.Aggregation_ALIGN_RATE
	// monitoringpb.Aggregation_ALIGN_PERCENTILE_99
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", monCon.Project),
		Filter: monCon.filter(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: startTime.Unix()},
			EndTime:   &timestamppb.Timestamp{Seconds: endtime.Unix()},
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  &durationpb.Duration{Seconds: 60},  // The value must be at least 60 seconds.
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_SUM, // sum for request count
			GroupByFields:    []string{"resource.revision_name"},
		},
		// PageSize: int32(10000), 100,000 if empty
	}
	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	cnt := Counter{}
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

		for i, p := range resp.GetPoints() { // Point per min
			log.Println(p.Value.String())
			log.Printf("Point:%d\tstart:%s\tend:%s\tvalue:%d\n", i, p.Interval.StartTime.AsTime(), p.Interval.EndTime.AsTime(), p.Value.GetInt64Value())
			val := p.GetValue().GetInt64Value()
			requestCount += val
			cnt[revision] += val
		}
		loopCnt++
	}
	log.Printf("Request count: %d, %s\n", requestCount, cnt)
	return &cnt, nil
}

func (mc *Client) Close() error {
	return mc.client.Close()
}
