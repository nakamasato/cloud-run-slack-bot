package config

import (
	"os"
	"testing"
)

func TestLoadConfig_MultiProject(t *testing.T) {
	// Set up environment variables
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	os.Setenv("SLACK_SIGNING_SECRET", "test-secret")
	os.Setenv("SLACK_CHANNEL", "default-channel")
	os.Setenv("PROJECTS_CONFIG", `[
		{
			"id": "project1",
			"region": "us-central1",
			"defaultChannel": "project1-channel",
			"serviceChannels": {
				"service1": "team1-channel"
			}
		},
		{
			"id": "project2",
			"region": "us-east1",
			"defaultChannel": "project2-channel"
		}
	]`)

	defer func() {
		os.Unsetenv("SLACK_BOT_TOKEN")
		os.Unsetenv("SLACK_SIGNING_SECRET")
		os.Unsetenv("SLACK_CHANNEL")
		os.Unsetenv("PROJECTS_CONFIG")
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(config.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(config.Projects))
	}

	if config.Projects[0].ID != "project1" {
		t.Errorf("Expected project1, got %s", config.Projects[0].ID)
	}

	if config.Projects[0].ServiceChannels["service1"] != "team1-channel" {
		t.Errorf("Expected team1-channel, got %s", config.Projects[0].ServiceChannels["service1"])
	}
}

func TestLoadConfig_LegacyMode(t *testing.T) {
	// Set up environment variables for legacy mode
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	os.Setenv("SLACK_SIGNING_SECRET", "test-secret")
	os.Setenv("PROJECT", "legacy-project")
	os.Setenv("REGION", "us-central1")
	os.Setenv("SLACK_CHANNEL", "default-channel")
	os.Setenv("SERVICE_CHANNEL_MAPPING", "service1:team1,service2:team2")

	defer func() {
		os.Unsetenv("SLACK_BOT_TOKEN")
		os.Unsetenv("SLACK_SIGNING_SECRET")
		os.Unsetenv("PROJECT")
		os.Unsetenv("REGION")
		os.Unsetenv("SLACK_CHANNEL")
		os.Unsetenv("SERVICE_CHANNEL_MAPPING")
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(config.Projects) != 1 {
		t.Errorf("Expected 1 project, got %d", len(config.Projects))
	}

	if config.Projects[0].ID != "legacy-project" {
		t.Errorf("Expected legacy-project, got %s", config.Projects[0].ID)
	}

	if config.Projects[0].ServiceChannels["service1"] != "team1" {
		t.Errorf("Expected team1, got %s", config.Projects[0].ServiceChannels["service1"])
	}

	if config.Projects[0].ServiceChannels["service2"] != "team2" {
		t.Errorf("Expected team2, got %s", config.Projects[0].ServiceChannels["service2"])
	}
}

func TestGetChannelForService(t *testing.T) {
	config := &Config{
		DefaultChannel: "global-default",
		Projects: []ProjectConfig{
			{
				ID:             "project1",
				Region:         "us-central1",
				DefaultChannel: "project1-default",
				ServiceChannels: map[string]string{
					"service1": "service1-channel",
				},
			},
			{
				ID:             "project2",
				Region:         "us-east1",
				DefaultChannel: "project2-default",
			},
		},
		ChannelToProjects: make(map[string][]string),
	}
	config.buildChannelToProjectMapping()

	tests := []struct {
		name        string
		projectID   string
		serviceName string
		expected    string
	}{
		{
			name:        "service with specific channel",
			projectID:   "project1",
			serviceName: "service1",
			expected:    "service1-channel",
		},
		{
			name:        "service with project default",
			projectID:   "project1",
			serviceName: "service2",
			expected:    "project1-default",
		},
		{
			name:        "service with global default",
			projectID:   "project2",
			serviceName: "service1",
			expected:    "project2-default",
		},
		{
			name:        "unknown project",
			projectID:   "unknown",
			serviceName: "service1",
			expected:    "global-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetChannelForService(tt.projectID, tt.serviceName)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				SlackBotToken:      "test-token",
				SlackSigningSecret: "test-secret",
				Projects: []ProjectConfig{
					{
						ID:     "project1",
						Region: "us-central1",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "missing bot token",
			config: &Config{
				SlackSigningSecret: "test-secret",
				Projects: []ProjectConfig{
					{
						ID:     "project1",
						Region: "us-central1",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "missing signing secret for http mode",
			config: &Config{
				SlackBotToken: "test-token",
				SlackAppMode:  "http",
				Projects: []ProjectConfig{
					{
						ID:     "project1",
						Region: "us-central1",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "socket mode without signing secret",
			config: &Config{
				SlackBotToken: "test-token",
				SlackAppMode:  "socket",
				Projects: []ProjectConfig{
					{
						ID:     "project1",
						Region: "us-central1",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "no projects configured",
			config: &Config{
				SlackBotToken:      "test-token",
				SlackSigningSecret: "test-secret",
				Projects:           []ProjectConfig{},
			},
			expectErr: true,
		},
		{
			name: "project missing ID",
			config: &Config{
				SlackBotToken:      "test-token",
				SlackSigningSecret: "test-secret",
				Projects: []ProjectConfig{
					{
						Region: "us-central1",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "project missing region",
			config: &Config{
				SlackBotToken:      "test-token",
				SlackSigningSecret: "test-secret",
				Projects: []ProjectConfig{
					{
						ID: "project1",
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestGetProjectsForChannel(t *testing.T) {
	config := &Config{
		DefaultChannel: "global-default",
		Projects: []ProjectConfig{
			{
				ID:             "project1",
				Region:         "us-central1",
				DefaultChannel: "project1-default",
				ServiceChannels: map[string]string{
					"service1": "service1-channel",
				},
			},
			{
				ID:             "project2",
				Region:         "us-east1",
				DefaultChannel: "project2-default",
			},
			{
				ID:             "project3",
				Region:         "us-west1",
				DefaultChannel: "shared-channel",
			},
		},
		ChannelToProjects: make(map[string][]string),
	}
	config.buildChannelToProjectMapping()

	tests := []struct {
		name     string
		channel  string
		expected []string
	}{
		{
			name:     "single project channel",
			channel:  "project1-default",
			expected: []string{"project1"},
		},
		{
			name:     "service-specific channel",
			channel:  "service1-channel",
			expected: []string{"project1"},
		},
		{
			name:     "single project shared channel",
			channel:  "shared-channel",
			expected: []string{"project3"},
		},
		{
			name:     "unknown channel",
			channel:  "unknown-channel",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetProjectsForChannel(tt.channel)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d projects, got %d", len(tt.expected), len(result))
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected project %s, got %s", expected, result[i])
				}
			}
		})
	}
}

func TestBuildChannelToProjectMapping(t *testing.T) {
	config := &Config{
		DefaultChannel: "global-default",
		Projects: []ProjectConfig{
			{
				ID:             "project1",
				Region:         "us-central1",
				DefaultChannel: "project1-channel",
				ServiceChannels: map[string]string{
					"service1": "service1-channel",
					"service2": "shared-channel",
				},
			},
			{
				ID:             "project2",
				Region:         "us-east1",
				DefaultChannel: "project2-channel",
				ServiceChannels: map[string]string{
					"service3": "shared-channel",
				},
			},
		},
		ChannelToProjects: make(map[string][]string),
	}
	config.buildChannelToProjectMapping()

	// Test single project mappings
	if projects := config.GetProjectsForChannel("project1-channel"); len(projects) != 1 || projects[0] != "project1" {
		t.Errorf("Expected project1-channel to map to [project1], got %v", projects)
	}

	if projects := config.GetProjectsForChannel("service1-channel"); len(projects) != 1 || projects[0] != "project1" {
		t.Errorf("Expected service1-channel to map to [project1], got %v", projects)
	}

	// Test multi-project mapping
	sharedProjects := config.GetProjectsForChannel("shared-channel")
	if len(sharedProjects) != 2 {
		t.Errorf("Expected shared-channel to map to 2 projects, got %d", len(sharedProjects))
	}

	expectedProjects := map[string]bool{"project1": true, "project2": true}
	for _, project := range sharedProjects {
		if !expectedProjects[project] {
			t.Errorf("Unexpected project %s in shared-channel mapping", project)
		}
	}
}
