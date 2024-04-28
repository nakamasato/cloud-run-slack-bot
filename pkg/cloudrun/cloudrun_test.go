package cloudrun

import (
	"testing"
)

func TestCloudRunService_GetMetricsUrl(t *testing.T) {
	tests := []struct {
		name string
		c    *CloudRunService
		want string
	}{
		{
			name: "test",
			c: &CloudRunService{
				Name:    "test",
				Region:  "asia-northeast1",
				Project: "project",
			},
			want: "https://console.cloud.google.com/run/detail/asia-northeast1/test/metrics?project=project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.GetMetricsUrl(); got != tt.want {
				t.Errorf("CloudRunService.GetMetricsUrl() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCloudRunService_GetYamlUrl(t *testing.T) {
	tests := []struct {
		name string
		c    *CloudRunService
		want string
	}{
		{
			name: "test",
			c: &CloudRunService{
				Name:    "test",
				Region:  "asia-northeast1",
				Project: "project",
			},
			want: "https://console.cloud.google.com/run/detail/asia-northeast1/test/yaml?project=project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.GetYamlUrl(); got != tt.want {
				t.Errorf("CloudRunService.GetYamlUrl() = %v, want %v", got, tt.want)
			}
		})
	}
}
