package cloudrun

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
	run "google.golang.org/api/run/v2"
)

type Client struct {
	project                      string
	region                       string
	runService                   *run.Service
	projectLocationServiceClient *run.ProjectsLocationsServicesService
	projectLocationJobClient     *run.ProjectsLocationsJobsService
	logger                       *zap.Logger
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

type CloudRunJob struct {
	Name         string
	Region       string
	Project      string
	Image        string
	LastModifier string
	UpdateTime   time.Time
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

func (c *CloudRunJob) GetYamlUrl() string {
	return c.getUrl("yaml")
}

// https://console.cloud.google.com/run/jobs/details/asia-northeast1/my-job/<urlPath>?project=<project>
// Supported urlPath: yaml, logs, executions, integrations
func (c *CloudRunJob) getUrl(urlPath string) string {
	return fmt.Sprintf("https://console.cloud.google.com/run/jobs/details/%s/%s/%s?project=%s", c.Region, c.Name, urlPath, c.Project)
}

func (c *CloudRunJob) String() string {
	return fmt.Sprintf(
		"Name: %s\n- Image: %s\n- LastModifier: %s\n- UpdateTime: %s\n- Resource Limit: (cpu:%s, memory:%s)\n",
		c.Name, c.Image, c.LastModifier, c.UpdateTime, c.ResourceLimits["cpu"], c.ResourceLimits["memory"],
	)
}

func (c *Client) getProjectLocation() string {
	return fmt.Sprintf("projects/%s/locations/%s", c.project, c.region)
}

func (c *Client) GetServiceNameFromFullname(fullname string) string {
	return strings.TrimPrefix(fullname, fmt.Sprintf("%s/services/", c.getProjectLocation()))
}

func (c *Client) GetJobNameFromFullname(fullname string) string {
	return strings.TrimPrefix(fullname, fmt.Sprintf("%s/jobs/", c.getProjectLocation()))
}

func NewClient(ctx context.Context, project, region string, logger *zap.Logger) (*Client, error) {
	runService, err := run.NewService(ctx)
	if err != nil {
		return nil, err
	}
	plSvc := run.NewProjectsLocationsServicesService(runService)
	plJobSvc := run.NewProjectsLocationsJobsService(runService)
	return &Client{
		project:                      project,
		region:                       region,
		runService:                   runService,
		projectLocationServiceClient: plSvc,
		projectLocationJobClient:     plJobSvc,
		logger:                       logger,
	}, nil
}

// Close closes the underlying HTTP client
func (c *Client) Close() error {
	c.runService.BasePath = ""
	return nil
}

func (c *Client) ListServices(ctx context.Context) ([]string, error) {
	ctx, span := trace.GetTracer().Start(ctx, "cloudrun.ListServices")
	defer span.End()

	span.SetAttributes(
		attribute.String("cloudrun.project", c.project),
		attribute.String("cloudrun.region", c.region),
	)

	projLoc := c.getProjectLocation()
	c.logger.Info("Listing services", zap.String("location", projLoc))
	res, err := c.projectLocationServiceClient.List(projLoc).Context(ctx).Do()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	var services []string
	for _, s := range res.Services {
		svcName := c.GetServiceNameFromFullname(s.Name)
		services = append(services, svcName)
	}
	span.SetAttributes(attribute.Int("cloudrun.services.count", len(services)))
	return services, nil
}

func (c *Client) ListJobs(ctx context.Context) ([]string, error) {
	ctx, span := trace.GetTracer().Start(ctx, "cloudrun.ListJobs")
	defer span.End()

	span.SetAttributes(
		attribute.String("cloudrun.project", c.project),
		attribute.String("cloudrun.region", c.region),
	)

	projLoc := c.getProjectLocation()
	c.logger.Info("Listing jobs", zap.String("location", projLoc))
	res, err := c.projectLocationJobClient.List(projLoc).Context(ctx).Do()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	var jobs []string
	for _, j := range res.Jobs {
		jobName := c.GetJobNameFromFullname(j.Name)
		jobs = append(jobs, jobName)
	}
	span.SetAttributes(attribute.Int("cloudrun.jobs.count", len(jobs)))
	return jobs, nil
}

func (c *Client) GetService(ctx context.Context, serviceName string) (*CloudRunService, error) {
	ctx, span := trace.GetTracer().Start(ctx, "cloudrun.GetService")
	defer span.End()

	span.SetAttributes(
		attribute.String("cloudrun.project", c.project),
		attribute.String("cloudrun.region", c.region),
		attribute.String("cloudrun.service.name", serviceName),
	)

	projLoc := c.getProjectLocation()
	res, err := c.projectLocationServiceClient.Get(fmt.Sprintf("%s/services/%s", projLoc, serviceName)).Context(ctx).Do()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	c.logger.Debug("Retrieved service", zap.String("service", serviceName), zap.Any("response", res))

	updateTime, err := time.Parse(time.RFC3339Nano, res.UpdateTime) // 2024-04-27T00:56:09.929299Z
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	service := &CloudRunService{
		Name:           c.GetServiceNameFromFullname(res.Name),
		Region:         c.region,
		Project:        c.project,
		Image:          res.Template.Containers[0].Image,
		ResourceLimits: res.Template.Containers[0].Resources.Limits,
		LastModifier:   res.LastModifier,
		UpdateTime:     updateTime,
		LatestRevision: strings.TrimPrefix(res.LatestCreatedRevision, fmt.Sprintf("%s/services/%s/revisions/", projLoc, serviceName)),
	}

	span.SetAttributes(
		attribute.String("cloudrun.service.image", service.Image),
		attribute.String("cloudrun.service.latest_revision", service.LatestRevision),
	)

	return service, nil
}

func (c *Client) GetJob(ctx context.Context, jobName string) (*CloudRunJob, error) {
	ctx, span := trace.GetTracer().Start(ctx, "cloudrun.GetJob")
	defer span.End()

	span.SetAttributes(
		attribute.String("cloudrun.project", c.project),
		attribute.String("cloudrun.region", c.region),
		attribute.String("cloudrun.job.name", jobName),
	)

	projLoc := c.getProjectLocation()
	res, err := c.projectLocationJobClient.Get(fmt.Sprintf("%s/jobs/%s", projLoc, jobName)).Context(ctx).Do()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	c.logger.Debug("Retrieved job", zap.String("job", jobName), zap.Any("response", res))

	updateTime, err := time.Parse(time.RFC3339Nano, res.UpdateTime)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	job := &CloudRunJob{
		Name:           c.GetJobNameFromFullname(res.Name),
		Region:         c.region,
		Project:        c.project,
		Image:          res.Template.Template.Containers[0].Image,
		ResourceLimits: res.Template.Template.Containers[0].Resources.Limits,
		LastModifier:   res.LastModifier,
		UpdateTime:     updateTime,
	}

	span.SetAttributes(attribute.String("cloudrun.job.image", job.Image))

	return job, nil
}
