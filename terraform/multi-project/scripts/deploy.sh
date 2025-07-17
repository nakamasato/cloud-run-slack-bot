#!/bin/bash

# Multi-Project Cloud Run Slack Bot Deployment Script

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}=== $1 ===${NC}"
}

# Check if terraform.tfvars exists
if [ ! -f "terraform.tfvars" ]; then
    print_error "terraform.tfvars not found!"
    print_status "Please copy terraform.tfvars.example to terraform.tfvars and configure it"
    exit 1
fi

# Check if required tools are installed
check_tools() {
    print_header "Checking required tools"

    if ! command -v terraform &> /dev/null; then
        print_error "Terraform is not installed"
        exit 1
    fi

    if ! command -v gcloud &> /dev/null; then
        print_error "Google Cloud SDK is not installed"
        exit 1
    fi

    print_status "Required tools are installed"
}

# Validate terraform configuration
validate_terraform() {
    print_header "Validating Terraform configuration"

    terraform fmt -check=true
    if [ $? -ne 0 ]; then
        print_warning "Running terraform fmt to fix formatting"
        terraform fmt
    fi

    terraform validate
    if [ $? -ne 0 ]; then
        print_error "Terraform validation failed"
        exit 1
    fi

    print_status "Terraform configuration is valid"
}

# Check GCP authentication
check_gcp_auth() {
    print_header "Checking GCP authentication"

    if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q .; then
        print_error "No active GCP authentication found"
        print_status "Please run: gcloud auth login"
        exit 1
    fi

    print_status "GCP authentication is active"
}

# Extract project IDs from terraform.tfvars
extract_projects() {
    print_header "Extracting project information"

    # Get host project ID
    HOST_PROJECT=$(grep -E '^host_project_id' terraform.tfvars | sed 's/.*= *"\(.*\)".*/\1/')
    if [ -z "$HOST_PROJECT" ]; then
        print_error "Could not extract host_project_id from terraform.tfvars"
        exit 1
    fi

    print_status "Host project: $HOST_PROJECT"

    # Get monitored projects (simplified extraction)
    MONITORED_PROJECTS=$(grep -E 'project_id.*=' terraform.tfvars | sed 's/.*= *"\(.*\)".*/\1/' | grep -v "$HOST_PROJECT")

    if [ -z "$MONITORED_PROJECTS" ]; then
        print_error "Could not extract monitored projects from terraform.tfvars"
        exit 1
    fi

    print_status "Monitored projects:"
    echo "$MONITORED_PROJECTS" | while read -r project; do
        echo "  - $project"
    done
}

# Enable required APIs
enable_apis() {
    print_header "Enabling required APIs"

    # APIs for host project
    HOST_APIS=(
        "run.googleapis.com"
        "cloudbuild.googleapis.com"
        "secretmanager.googleapis.com"
        "pubsub.googleapis.com"
        "logging.googleapis.com"
        "monitoring.googleapis.com"
        "iam.googleapis.com"
    )

    print_status "Enabling APIs in host project: $HOST_PROJECT"
    for api in "${HOST_APIS[@]}"; do
        print_status "  Enabling $api"
        gcloud services enable "$api" --project="$HOST_PROJECT"
    done

    # APIs for monitored projects
    MONITORED_APIS=(
        "run.googleapis.com"
        "monitoring.googleapis.com"
        "logging.googleapis.com"
        "pubsub.googleapis.com"
    )

    echo "$MONITORED_PROJECTS" | while read -r project; do
        if [ -n "$project" ]; then
            print_status "Enabling APIs in monitored project: $project"
            for api in "${MONITORED_APIS[@]}"; do
                print_status "  Enabling $api"
                gcloud services enable "$api" --project="$project"
            done
        fi
    done
}

# Deploy with terraform
deploy_terraform() {
    print_header "Deploying with Terraform"

    # Initialize terraform
    print_status "Initializing Terraform"
    terraform init

    # Create terraform plan
    print_status "Creating Terraform plan"
    terraform plan -out=tfplan

    # Ask for confirmation
    echo
    print_warning "Ready to deploy. This will create resources in multiple GCP projects."
    read -p "Do you want to continue? (y/N): " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_status "Deployment cancelled"
        exit 0
    fi

    # Apply terraform
    print_status "Applying Terraform configuration"
    terraform apply tfplan

    # Clean up plan file
    rm -f tfplan
}

# Display deployment results
show_results() {
    print_header "Deployment Results"

    # Get outputs
    SERVICE_URL=$(terraform output -raw service_url)
    WEBHOOK_URL=$(terraform output -raw webhook_url)
    INTERACTION_URL=$(terraform output -raw interaction_url)

    print_status "Deployment completed successfully!"
    echo
    print_status "Service URL: $SERVICE_URL"
    print_status "Webhook URL: $WEBHOOK_URL"
    print_status "Interaction URL: $INTERACTION_URL"
    echo

    print_header "Next Steps"
    echo "1. Configure your Slack App with these URLs:"
    echo "   - Events API URL: $WEBHOOK_URL"
    echo "   - Interactive Components URL: $INTERACTION_URL"
    echo
    echo "2. Invite the bot to your Slack channels"
    echo
    echo "3. Test the bot:"
    echo "   - Try: @cloud-run-slack-bot help"
    echo
    echo "4. Monitor the deployment:"
    echo "   - Cloud Run: https://console.cloud.google.com/run?project=$HOST_PROJECT"
    echo "   - Logs: https://console.cloud.google.com/logs?project=$HOST_PROJECT"
    echo

    print_status "For more information, see the README.md file"
}

# Main deployment function
main() {
    print_header "Multi-Project Cloud Run Slack Bot Deployment"

    check_tools
    check_gcp_auth
    validate_terraform
    extract_projects

    # Ask if user wants to enable APIs
    echo
    print_warning "This script will enable required APIs in all projects."
    read -p "Do you want to enable APIs automatically? (y/N): " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        enable_apis
    else
        print_status "Skipping API enablement. Make sure required APIs are enabled manually."
    fi

    deploy_terraform
    show_results
}

# Script options
case "${1:-}" in
    "plan")
        check_tools
        check_gcp_auth
        validate_terraform
        terraform plan
        ;;
    "destroy")
        print_header "Destroying infrastructure"
        print_warning "This will destroy all resources created by Terraform"
        read -p "Are you sure? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            terraform destroy
        else
            print_status "Destruction cancelled"
        fi
        ;;
    "validate")
        check_tools
        validate_terraform
        ;;
    *)
        main
        ;;
esac
