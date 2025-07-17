terraform {
  required_version = ">= 1.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }

  # Optional: Configure backend for state management
  # backend "gcs" {
  #   bucket = "your-terraform-state-bucket"
  #   prefix = "cloud-run-slack-bot"
  # }
}
