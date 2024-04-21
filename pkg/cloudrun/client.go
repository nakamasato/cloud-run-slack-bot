package cloudrun

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/run/v2"
)

type Client struct {
	project                      string
	region                       string
	projectLocationServiceClient *run.ProjectsLocationsServicesService
}

type CloudRunService struct {
	Name           string
	LastModifier   string
	UpdateTime     string
	LatestRevision string
}

func (c *CloudRunService) String() string {
	return fmt.Sprintf("Name: %s\nLatestRevision: %s\nLastModifier: %s\nUpdateTime: %s", c.Name, c.LatestRevision, c.LastModifier, c.UpdateTime)
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

	return &CloudRunService{
		Name:           c.GetServiceNameFromFullname(res.Name),
		LastModifier:   res.LastModifier,
		UpdateTime:     res.UpdateTime,
		LatestRevision: strings.TrimPrefix(res.LatestCreatedRevision, fmt.Sprintf("%s/services/%s/revisions/", projLoc, serviceName)),
	}, nil
}
