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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MonitorFilter map[string]string

type MonitorCondition struct {
	Project string // used for name in ListTimeSeriesRequest
	Filters []MonitorFilter
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

func (mc *Client) GetCloudRunServiceRequestCount(ctx context.Context, service string, duration time.Duration) (int64, error) {
	monCon := MonitorCondition{
		Project: mc.project,
		Filters: []MonitorFilter{
			{"resource.labels.service_name": service},
			{"metric.type": "run.googleapis.com/request_count"},
		},
	}
	return mc.GetRequestCount(ctx, monCon, duration)
}

func (mc *Client) GetRequestCount(ctx context.Context, monCon MonitorCondition, duration time.Duration) (int64, error) {
	endtime := time.Now().UTC()
	startTime := endtime.Add(-1 * duration).UTC()
	// See https://pkg.go.dev/cloud.google.com/go/monitoring/apiv3/v2/monitoringpb#ListTimeSeriesRequest.
	fmt.Printf("[%s] get metrics %s (%s -> %s)\n", monCon.Project, monCon.filter(), startTime, endtime)
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", monCon.Project),
		Filter: monCon.filter(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: startTime.Unix()},
			EndTime:   &timestamppb.Timestamp{Seconds: endtime.Unix()},
		},
		PageSize: int32(10000),
	}
	it := mc.client.ListTimeSeries(ctx, req)
	var requestCount int64
	var loopCnt int
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			fmt.Printf("iterator.Done %d\n", loopCnt)
			break
		}
		if err != nil {
			fmt.Printf("err %v\n", err)
			return 0, err
		}
		if resp == nil {
			fmt.Println("nil")
			continue
		}
		for i, p := range resp.GetPoints() { // Point per min
			fmt.Printf("Point:%d\tstart:%s\tend:%s\tvalue:%d\n", i, p.Interval.StartTime.AsTime(), p.Interval.EndTime.AsTime(), p.Value.GetInt64Value())
			requestCount += p.GetValue().GetInt64Value()
		}
		loopCnt++
	}
	fmt.Printf("Request count: %d\n", requestCount)
	return requestCount, nil
}

func (mc *Client) Close() error {
	return mc.client.Close()
}
