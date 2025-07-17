output "service_url" {
  description = "URL of the Cloud Run service"
  value       = google_cloud_run_v2_service.bot.uri
}

output "service_account_email" {
  description = "Email of the service account used by the bot"
  value       = google_service_account.bot_sa.email
}

output "webhook_url" {
  description = "Webhook URL for Slack Events API configuration"
  value       = "${google_cloud_run_v2_service.bot.uri}/slack/events"
}

output "interaction_url" {
  description = "Interaction URL for Slack interactive components"
  value       = "${google_cloud_run_v2_service.bot.uri}/slack/interaction"
}

output "audit_webhook_url" {
  description = "Webhook URL for Cloud Run audit logs"
  value       = "${google_cloud_run_v2_service.bot.uri}/cloudrun/events"
}

output "audit_topics" {
  description = "Pub/Sub topics for audit logs by project"
  value = {
    for project_id, topic in google_pubsub_topic.audit_logs : project_id => topic.id
  }
}

output "audit_subscriptions" {
  description = "Pub/Sub subscriptions for audit logs by project"
  value = {
    for project_id, subscription in google_pubsub_subscription.audit_subscription : project_id => subscription.id
  }
}

output "projects_config" {
  description = "Generated projects configuration JSON"
  value = jsonencode([
    for project in var.monitored_projects : {
      id              = project.project_id
      region          = project.region
      defaultChannel  = project.default_channel
      serviceChannels = project.service_channels
    }
  ])
  sensitive = false
}

output "monitored_projects" {
  description = "List of monitored projects with their configurations"
  value = {
    for project in var.monitored_projects : project.project_id => {
      region          = project.region
      default_channel = project.default_channel
      service_channels = project.service_channels
    }
  }
}

output "channel_to_project_mapping" {
  description = "Channel to project mapping for reference"
  value = {
    for project in var.monitored_projects : project.project_id => {
      default_channel = project.default_channel
      service_channels = project.service_channels
    }
  }
}

output "setup_instructions" {
  description = "Next steps for completing the setup"
  value = <<-EOT

    🎉 Terraform deployment completed successfully!

    Next steps:
    1. Configure Slack App:
       - Events API URL: ${google_cloud_run_v2_service.bot.uri}/slack/events
       - Interactive Components URL: ${google_cloud_run_v2_service.bot.uri}/slack/interaction

    2. Test the bot:
       - Invite the bot to your Slack channels
       - Try: @${var.cloud_run_service_name} help

    3. Monitor the logs:
       - Cloud Run logs: https://console.cloud.google.com/run/detail/${var.service_region}/${var.cloud_run_service_name}/logs?project=${var.host_project_id}
       - Pub/Sub monitoring: https://console.cloud.google.com/cloudpubsub/subscription/list?project=${var.host_project_id}

    4. Channel-to-Project Mapping:
       ${join("\n       ", [for project in var.monitored_projects : "${project.default_channel} → ${project.project_id} (auto-detect enabled)"])}

    EOT
}
