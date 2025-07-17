package slack

import (
	"testing"
)

func TestParseResourceValue(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		expectedType   string
		expectedName   string
		expectedError  bool
	}{
		{
			name:         "valid service format",
			value:        "service:my-service",
			expectedType: "service",
			expectedName: "my-service",
			expectedError: false,
		},
		{
			name:         "valid job format",
			value:        "job:my-job",
			expectedType: "job",
			expectedName: "my-job",
			expectedError: false,
		},
		{
			name:         "legacy format without type",
			value:        "my-service",
			expectedType: "service",
			expectedName: "my-service",
			expectedError: false,
		},
		{
			name:         "empty value",
			value:        "",
			expectedError: true,
		},
		{
			name:         "invalid resource type",
			value:        "invalid:my-service",
			expectedError: true,
		},
		{
			name:         "empty resource name",
			value:        "service:",
			expectedError: true,
		},
		{
			name:         "malformed format",
			value:        "service:name:extra",
			expectedType: "service",
			expectedName: "name:extra",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceType, resourceName, err := ParseResourceValue(tt.value)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resourceType != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, resourceType)
			}

			if resourceName != tt.expectedName {
				t.Errorf("expected name %q, got %q", tt.expectedName, resourceName)
			}
		})
	}
}

func TestParseMultiProjectResourceValue(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		expectedProject string
		expectedType   string
		expectedName   string
		expectedError  bool
	}{
		{
			name:           "valid multi-project service",
			value:          "my-project:service:my-service",
			expectedProject: "my-project",
			expectedType:   "service",
			expectedName:   "my-service",
			expectedError:  false,
		},
		{
			name:           "valid multi-project job",
			value:          "my-project:job:my-job",
			expectedProject: "my-project",
			expectedType:   "job",
			expectedName:   "my-job",
			expectedError:  false,
		},
		{
			name:         "empty value",
			value:        "",
			expectedError: true,
		},
		{
			name:         "invalid format - only two parts",
			value:        "my-project:service",
			expectedError: true,
		},
		{
			name:         "invalid format - only one part",
			value:        "my-project",
			expectedError: true,
		},
		{
			name:         "empty project ID",
			value:        ":service:my-service",
			expectedError: true,
		},
		{
			name:         "invalid resource type",
			value:        "my-project:invalid:my-service",
			expectedError: true,
		},
		{
			name:         "empty resource name",
			value:        "my-project:service:",
			expectedError: true,
		},
		{
			name:           "resource name with colons",
			value:          "my-project:service:my-service:with:colons",
			expectedProject: "my-project",
			expectedType:   "service",
			expectedName:   "my-service:with:colons",
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectID, resourceType, resourceName, err := ParseMultiProjectResourceValue(tt.value)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if projectID != tt.expectedProject {
				t.Errorf("expected project %q, got %q", tt.expectedProject, projectID)
			}

			if resourceType != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, resourceType)
			}

			if resourceName != tt.expectedName {
				t.Errorf("expected name %q, got %q", tt.expectedName, resourceName)
			}
		})
	}
}
