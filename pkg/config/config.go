package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
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

	// Check for multi-project configuration
	if projectsConfig := os.Getenv("PROJECTS_CONFIG"); projectsConfig != "" {
		if err := json.Unmarshal([]byte(projectsConfig), &config.Projects); err != nil {
			return nil, fmt.Errorf("failed to parse PROJECTS_CONFIG: %v", err)
		}
	} else {
		// Fallback to single project configuration for backward compatibility
		project := os.Getenv("PROJECT")
		if project == "" {
			return nil, fmt.Errorf("PROJECT env var is required when PROJECTS_CONFIG is not set")
		}

		region := os.Getenv("REGION")
		if region == "" {
			return nil, fmt.Errorf("REGION env var is required when PROJECTS_CONFIG is not set")
		}

		// Parse service-channel mapping for backward compatibility
		serviceChannels := make(map[string]string)
		if serviceChannelStr := os.Getenv("SERVICE_CHANNEL_MAPPING"); serviceChannelStr != "" {
			pairs := strings.Split(serviceChannelStr, ",")
			for _, pair := range pairs {
				parts := strings.Split(pair, ":")
				if len(parts) == 2 {
					serviceChannels[parts[0]] = parts[1]
				}
			}
		}

		config.Projects = []ProjectConfig{
			{
				ID:              project,
				Region:          region,
				DefaultChannel:  config.DefaultChannel,
				ServiceChannels: serviceChannels,
			},
		}
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
func (c *Config) LogConfiguration() {
	log.Printf("Configuration loaded:")
	log.Printf("  Default Channel: %s", c.DefaultChannel)
	log.Printf("  Slack App Mode: %s", c.SlackAppMode)
	log.Printf("  Projects:")
	for _, project := range c.Projects {
		log.Printf("    - ID: %s, Region: %s, Default Channel: %s",
			project.ID, project.Region, project.DefaultChannel)
		if len(project.ServiceChannels) > 0 {
			log.Printf("      Service Channels: %v", project.ServiceChannels)
		}
	}
	log.Printf("  Channel-to-Project Mapping:")
	for channel, projects := range c.ChannelToProjects {
		if len(projects) == 1 {
			log.Printf("    - Channel '%s' -> Project '%s' (auto-detect enabled)", channel, projects[0])
		} else {
			log.Printf("    - Channel '%s' -> Projects %v (manual selection required)", channel, projects)
		}
	}
}
