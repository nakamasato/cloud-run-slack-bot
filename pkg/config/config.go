package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"go.uber.org/zap"
)

// ProjectConfig represents configuration for a single GCP project
type ProjectConfig struct {
	ID           string            `json:"id"`
	Region       string            `json:"region"`
	DefaultChannel string          `json:"defaultChannel"`
	ServiceChannels map[string]string `json:"serviceChannels"`
}

// Config represents the multi-project configuration
type Config struct {
	Projects              []ProjectConfig     `json:"projects"`
	DefaultChannel        string              `json:"defaultChannel"`
	ChannelToProjects     map[string][]string `json:"-"` // Maps channel names to project IDs (can be multiple)
	SlackBotToken         string              `json:"-"`
	SlackAppToken         string              `json:"-"`
	SlackSigningSecret    string              `json:"-"`
	SlackAppMode          string              `json:"-"`
	TmpDir                string              `json:"-"`

	// Debug feature configuration
	DebugEnabled    bool   `json:"-"`
	GCPProjectID    string `json:"-"` // GCP project for Vertex AI
	VertexLocation  string `json:"-"` // GCP location for Vertex AI
	ModelName       string `json:"-"` // Gemini model name
	DebugTimeWindow int    `json:"-"` // How far back to look for errors (minutes)
}

// validateProjectsConfig validates the structure of the parsed projects configuration
func validateProjectsConfig(projects []ProjectConfig) error {
	if len(projects) == 0 {
		return fmt.Errorf("at least one project must be configured")
	}

	for i, project := range projects {
		if project.ID == "" {
			return fmt.Errorf("project %d: project ID is required", i)
		}
		if project.Region == "" {
			return fmt.Errorf("project %d: region is required", i)
		}
		// DefaultChannel is optional
		// ServiceChannels is optional
	}
	return nil
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:      os.Getenv("SLACK_APP_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
		SlackAppMode:       os.Getenv("SLACK_APP_MODE"),
		TmpDir:             os.Getenv("TMP_DIR"),
		DefaultChannel:     os.Getenv("SLACK_CHANNEL"),
		ChannelToProjects:  make(map[string][]string),
	}

	// Load debug configuration
	config.DebugEnabled = os.Getenv("DEBUG_ENABLED") == "true"
	config.GCPProjectID = os.Getenv("GCP_PROJECT_ID")
	config.VertexLocation = os.Getenv("VERTEX_LOCATION")
	config.ModelName = os.Getenv("MODEL_NAME")
	if config.ModelName == "" {
		config.ModelName = "gemini-2.5-flash-lite"
	}
	if timeWindow := os.Getenv("DEBUG_TIME_WINDOW"); timeWindow != "" {
		if val, err := strconv.Atoi(timeWindow); err == nil {
			config.DebugTimeWindow = val
		}
	}
	if config.DebugTimeWindow == 0 {
		config.DebugTimeWindow = 30
	}

	// Check for multi-project configuration
	projectsConfig := os.Getenv("PROJECTS_CONFIG")
	if projectsConfig == "" {
		return nil, fmt.Errorf("PROJECTS_CONFIG env var is required")
	}

	if err := json.Unmarshal([]byte(projectsConfig), &config.Projects); err != nil {
		return nil, fmt.Errorf("failed to parse PROJECTS_CONFIG: %v", err)
	}

	// Validate the parsed configuration structure
	if err := validateProjectsConfig(config.Projects); err != nil {
		return nil, fmt.Errorf("invalid PROJECTS_CONFIG: %v", err)
	}

	// Build channel-to-project mapping
	config.buildChannelToProjectMapping()

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.SlackBotToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN is required")
	}

	if c.SlackSigningSecret == "" && c.SlackAppMode != "socket" {
		return fmt.Errorf("SLACK_SIGNING_SECRET is required for HTTP mode")
	}

	if len(c.Projects) == 0 {
		return fmt.Errorf("at least one project must be configured")
	}

	for i, project := range c.Projects {
		if project.ID == "" {
			return fmt.Errorf("project %d: project ID is required", i)
		}
		if project.Region == "" {
			return fmt.Errorf("project %d: region is required", i)
		}
	}

	// Validate debug configuration
	if c.DebugEnabled {
		if c.GCPProjectID == "" {
			return fmt.Errorf("GCP_PROJECT_ID is required when DEBUG_ENABLED=true")
		}
		if c.VertexLocation == "" {
			return fmt.Errorf("VERTEX_LOCATION is required when DEBUG_ENABLED=true")
		}
	}

	return nil
}

// buildChannelToProjectMapping builds the mapping from channels to projects
func (c *Config) buildChannelToProjectMapping() {
	for _, project := range c.Projects {
		// Add project default channel
		if project.DefaultChannel != "" {
			c.ChannelToProjects[project.DefaultChannel] = append(c.ChannelToProjects[project.DefaultChannel], project.ID)
		}

		// Add service-specific channels
		for _, channel := range project.ServiceChannels {
			if channel != "" {
				c.ChannelToProjects[channel] = append(c.ChannelToProjects[channel], project.ID)
			}
		}
	}

	// Remove duplicate project IDs for each channel
	for channel, projects := range c.ChannelToProjects {
		c.ChannelToProjects[channel] = removeDuplicates(projects)
	}
}

// removeDuplicates removes duplicate strings from a slice
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	result := []string{}
	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}
	return result
}

// GetProjectsForChannel returns the projects associated with a channel
func (c *Config) GetProjectsForChannel(channel string) []string {
	if projects, exists := c.ChannelToProjects[channel]; exists {
		return projects
	}
	return []string{}
}

// GetChannelForService returns the appropriate Slack channel for a service/job
func (c *Config) GetChannelForService(projectID, serviceName string) string {
	// Find the project configuration
	for _, project := range c.Projects {
		if project.ID == projectID {
			// Check if there's a specific channel for this service
			if channel, ok := project.ServiceChannels[serviceName]; ok {
				return channel
			}
			// Fall back to project default channel
			if project.DefaultChannel != "" {
				return project.DefaultChannel
			}
		}
	}

	// Fall back to global default channel
	return c.DefaultChannel
}

// GetProjectConfig returns the project configuration for the given project ID
func (c *Config) GetProjectConfig(projectID string) (*ProjectConfig, bool) {
	for _, project := range c.Projects {
		if project.ID == projectID {
			return &project, true
		}
	}
	return nil, false
}

// LogConfiguration logs the current configuration (without sensitive data)
func (c *Config) LogConfiguration(logger *zap.Logger) {
	logger.Info("Configuration loaded")
	logger.Info("Default Channel", zap.String("channel", c.DefaultChannel))
	logger.Info("Slack App Mode", zap.String("mode", c.SlackAppMode))
	logger.Info("Projects configured", zap.Int("count", len(c.Projects)))
	for _, project := range c.Projects {
		logger.Info("Project configuration",
			zap.String("project_id", project.ID),
			zap.String("region", project.Region),
			zap.String("default_channel", project.DefaultChannel))
		if len(project.ServiceChannels) > 0 {
			logger.Info("Service channels", zap.String("project_id", project.ID), zap.Any("channels", project.ServiceChannels))
		}
	}
	logger.Info("Channel-to-Project Mapping", zap.Int("channels", len(c.ChannelToProjects)))
	for channel, projects := range c.ChannelToProjects {
		if len(projects) == 1 {
			logger.Info("Channel mapping (auto-detect enabled)",
				zap.String("channel", channel),
				zap.String("project", projects[0]))
		} else {
			logger.Info("Channel mapping (manual selection required)",
				zap.String("channel", channel),
				zap.Strings("projects", projects))
		}
	}
	logger.Info("Debug feature configuration", zap.Bool("enabled", c.DebugEnabled))
	if c.DebugEnabled {
		logger.Info("Debug feature details",
			zap.String("gcp_project_id", c.GCPProjectID),
			zap.String("vertex_location", c.VertexLocation),
			zap.String("model", c.ModelName),
			zap.Int("time_window_minutes", c.DebugTimeWindow))
	}
}
