package cloudrun

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/run/v1"
)

type Client struct {
	project string
	region  string
	nsSvc   *run.NamespacesServicesService
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
	runSvc, err := run.NewService(ctx)
	runSvc.BasePath = fmt.Sprintf("https://%s-run.googleapis.com/", region)
	if err != nil {
		return nil, err
	}
	nsSvc := run.NewNamespacesServicesService(runSvc)
	return &Client{
		project: project,
		region:  region,
		nsSvc:   nsSvc,
	}, nil
}

func (c *Client) ListServices(ctx context.Context) ([]string, error) {
	// The parent from where the resources should be listed. In Cloud Run, it may be one of the following:
	// * `{project_id_or_number}`
	// * `namespaces/{project_id_or_number}`
	// * `namespaces/{project_id_or_number}/services`
	// * `projects/{project_id_or_number}/locations/{region}`
	// * `projects/{project_id_or_number}/regions/{region}`.
	res, err := c.nsSvc.List(fmt.Sprintf("namespaces/%s", c.project)).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var svcNames []string
	for _, i := range res.Items {
		svcName := c.GetServiceNameFromFullname(i.Metadata.Name)
		svcNames = append(svcNames, svcName)
	}
	return svcNames, nil
}

func (c *Client) GetService(ctx context.Context, serviceName string) (*CloudRunService, error) {
	svc, err := c.nsSvc.Get(fmt.Sprintf("namespaces/%s/services/%s", c.project, serviceName)).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	updateTime, err := time.Parse(time.RFC3339Nano, svc.Status.Conditions[0].LastTransitionTime) // 2024-04-27T00:56:09.929299Z
	if err != nil {
		return nil, err
	}

	return &CloudRunService{
		Name:           c.GetServiceNameFromFullname(svc.Metadata.Name),
		Region:         c.region,
		Project:        c.project,
		Image:          svc.Spec.Template.Spec.Containers[0].Image, // only first container
		ResourceLimits: svc.Spec.Template.Spec.Containers[0].Resources.Limits,
		LastModifier:   svc.Metadata.Annotations["serving.knative.dev/lastModifier"],
		UpdateTime:     updateTime,
		LatestRevision: svc.Status.LatestCreatedRevisionName,
	}, nil
}
