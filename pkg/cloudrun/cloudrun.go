package cloudrun

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/api/run/v2"
)

type Client struct {
	project                      string
	region                       string
	projectLocationServiceClient *run.ProjectsLocationsServicesService
}

type CloudRunService struct {
	Name           string
	Region         string
	Project        string
	Image          string
	LastModifier   string
	UpdateTime     time.Time
	LatestRevision string
	ResourceLimits map[string]string
}

func (c *CloudRunService) GetMetricsUrl() string {
	return c.getUrl("metrics")
}

func (c *CloudRunService) GetYamlUrl() string {
	return c.getUrl("yaml")
}

// https://console.cloud.google.com/run/detail/asia-northeast1/cloud-run-slack-bot/<urlPath>?project=<project>
// Supported urlPath: metrics, slos, logs, revisions, networking, triggers, integrations, yaml
func (c *CloudRunService) getUrl(urlPath string) string {
	return fmt.Sprintf("https://console.cloud.google.com/run/detail/%s/%s/%s?project=%s", c.Region, c.Name, urlPath, c.Project)
}

func (c *CloudRunService) String() string {
	return fmt.Sprintf(
		"Name: %s\n- LatestRevision: %s\n- Image: %s\n- LastModifier: %s\n- UpdateTime: %s\n- Resource Limit: (cpu:%s, memory:%s)\n",
		c.Name, c.LatestRevision, c.Image, c.LastModifier, c.UpdateTime, c.ResourceLimits["cpu"], c.ResourceLimits["memory"],
	)
}

func (c *Client) getProjectLocation() string {
	return fmt.Sprintf("projects/%s/locations/%s", c.project, c.region)
}

func (c *Client) GetServiceNameFromFullname(fullname string) string {
	return strings.TrimPrefix(fullname, fmt.Sprintf("%s/services/", c.getProjectLocation()))
}

func NewClient(ctx context.Context, project, region string) (*Client, error) {
	runService, err := run.NewService(ctx)
	if err != nil {
		return nil, err
	}
	plSvc := run.NewProjectsLocationsServicesService(runService)
	return &Client{
		project:                      project,
		region:                       region,
		projectLocationServiceClient: plSvc,
	}, nil
}

func (c *Client) ListServices(ctx context.Context) ([]string, error) {
	projLoc := c.getProjectLocation()
	log.Printf("Listing services in %s\n", projLoc)
	res, err := c.projectLocationServiceClient.List(projLoc).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var services []string
	for _, s := range res.Services {
		svcName := c.GetServiceNameFromFullname(s.Name)
		services = append(services, svcName)
	}
	return services, nil
}

func (c *Client) GetService(ctx context.Context, serviceName string) (*CloudRunService, error) {
	projLoc := c.getProjectLocation()
	res, err := c.projectLocationServiceClient.Get(fmt.Sprintf("%s/services/%s", projLoc, serviceName)).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	fmt.Printf("Service: %+v\n", res)

	updateTime, err := time.Parse(time.RFC3339Nano, res.UpdateTime) // 2024-04-27T00:56:09.929299Z
	if err != nil {
		return nil, err
	}

	return &CloudRunService{
		Name:           c.GetServiceNameFromFullname(res.Name),
		Region:         c.region,
		Project:        c.project,
		Image:          res.Template.Containers[0].Image,
		ResourceLimits: res.Template.Containers[0].Resources.Limits,
		LastModifier:   res.LastModifier,
		UpdateTime:     updateTime,
		LatestRevision: strings.TrimPrefix(res.LatestCreatedRevision, fmt.Sprintf("%s/services/%s/revisions/", projLoc, serviceName)),
	}, nil
}
