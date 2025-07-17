variable "host_project_id" {
  description = "Project ID where the bot will be deployed"
  type        = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.host_project_id))
    error_message = "Project ID must be a valid GCP project ID format."
  }
}

variable "monitored_projects" {
  description = "List of projects to monitor with their channel configurations"
  type = list(object({
    project_id      = string
    region          = string
    default_channel = string
    service_channels = map(string)
  }))
  validation {
    condition     = length(var.monitored_projects) > 0
    error_message = "At least one project must be configured for monitoring."
  }
}

variable "slack_bot_token" {
  description = "Slack bot token (xoxb-...)"
  type        = string
  sensitive   = true
  validation {
    condition     = can(regex("^xoxb-", var.slack_bot_token))
    error_message = "Slack bot token must start with 'xoxb-'."
  }
}

variable "slack_signing_secret" {
  description = "Slack signing secret for webhook verification"
  type        = string
  sensitive   = true
  validation {
    condition     = length(var.slack_signing_secret) > 0
    error_message = "Slack signing secret cannot be empty."
  }
}

variable "slack_app_token" {
  description = "Slack app token (xapp-...) for socket mode"
  type        = string
  sensitive   = true
  default     = ""
}

variable "slack_app_mode" {
  description = "Slack app mode: 'http' for Events API or 'socket' for Socket Mode"
  type        = string
  default     = "http"
  validation {
    condition     = contains(["http", "socket"], var.slack_app_mode)
    error_message = "Slack app mode must be either 'http' or 'socket'."
  }
}

variable "default_channel" {
  description = "Default Slack channel for notifications"
  type        = string
  default     = "general"
}

variable "service_account_name" {
  description = "Name of the service account for the bot"
  type        = string
  default     = "cloud-run-slack-bot"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{4,28}[a-z0-9]$", var.service_account_name))
    error_message = "Service account name must be a valid format (6-30 characters, lowercase letters, numbers, and hyphens)."
  }
}

variable "cloud_run_service_name" {
  description = "Name of the Cloud Run service"
  type        = string
  default     = "cloud-run-slack-bot"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,61}[a-z0-9]$", var.cloud_run_service_name))
    error_message = "Cloud Run service name must be a valid format (1-63 characters, lowercase letters, numbers, and hyphens)."
  }
}

variable "container_image" {
  description = "Container image for the bot (PROJECT_ID will be replaced with host_project_id)"
  type        = string
  default     = "gcr.io/PROJECT_ID/cloud-run-slack-bot:latest"
}

variable "service_region" {
  description = "Region where the Cloud Run service will be deployed"
  type        = string
  default     = "us-central1"
}

variable "min_instances" {
  description = "Minimum number of instances for the Cloud Run service"
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum number of instances for the Cloud Run service"
  type        = number
  default     = 10
}

variable "cpu_limit" {
  description = "CPU limit for the Cloud Run service"
  type        = string
  default     = "1000m"
}

variable "memory_limit" {
  description = "Memory limit for the Cloud Run service"
  type        = string
  default     = "1Gi"
}
