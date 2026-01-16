// Package adk provides a GenAI client wrapper for error analysis using Gemini via Vertex AI.
package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/genai"
)

const maxErrorsForGrouping = 100 // Limit errors to prevent LLM context window issues

// extractJSON extracts a JSON string from a markdown code block or raw response.
func extractJSON(response string) string {
	// Regex to find content within ```json ... ``` or ``` ... ```
	re := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)\\s*```")
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	// Fallback for raw JSON response without markdown
	return strings.TrimSpace(response)
}

// Config for agent initialization.
type Config struct {
	Project   string // GCP project for Vertex AI
	Location  string // GCP location (e.g., "us-central1")
	ModelName string // Gemini model (e.g., "gemini-2.5-flash")
}

// ErrorLog is input for error grouping.
type ErrorLog struct {
	Message   string
	Timestamp time.Time
	TraceID   string
}

// ErrorGroup represents a group of similar errors.
type ErrorGroup struct {
	Pattern        string     // Pattern describing this group
	Representative ErrorLog   // Representative error for this group
	SimilarErrors  []ErrorLog // Other errors in this group
	Count          int        // Total count of errors in this group
}

// ErrorAnalysis is the LLM analysis result.
type ErrorAnalysis struct {
	Summary        string   // Brief summary of the error group
	PossibleCauses []string // Possible root causes
	Suggestions    []string // Actionable suggestions
}

// DebugAgent wraps the GenAI client for error analysis.
type DebugAgent struct {
	client *genai.Client
	model  string
	logger *zap.Logger
}

// NewDebugAgent creates a new agent configured for Vertex AI.
func NewDebugAgent(ctx context.Context, cfg Config, logger *zap.Logger) (*DebugAgent, error) {
	clientConfig := &genai.ClientConfig{
		Project:  cfg.Project,
		Location: cfg.Location,
		Backend:  genai.BackendVertexAI,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create GenAI client: %w", err)
	}

	logger.Info("GenAI agent created",
		zap.String("model", cfg.ModelName),
		zap.String("project", cfg.Project),
		zap.String("location", cfg.Location))
	return &DebugAgent{client: client, model: cfg.ModelName, logger: logger}, nil
}

var groupResponseSchema = &genai.Schema{
	Type: genai.TypeArray,
	Items: &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"pattern": {
				Type:        genai.TypeString,
				Description: "A brief description of the error pattern.",
			},
			"indices": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeInteger,
				},
				Description: "1-based indices of errors belonging to this group.",
			},
		},
		Required: []string{"pattern", "indices"},
	},
}

var analysisResponseSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"summary": {
			Type:        genai.TypeString,
			Description: "Brief summary of what's happening (1-2 sentences).",
		},
		"possible_causes": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeString,
			},
			Description: "2-4 possible root causes.",
		},
		"suggestions": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeString,
			},
			Description: "2-4 actionable suggestions to fix or investigate.",
		},
	},
	Required: []string{"summary", "possible_causes", "suggestions"},
}

// generateContent is a helper method to generate content from the LLM.
func (a *DebugAgent) generateContent(ctx context.Context, prompt string, outputSchema *genai.Schema) (string, error) {
	var cfg *genai.GenerateContentConfig
	if outputSchema != nil {
		cfg = &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   outputSchema,
		}
	}

	result, err := a.client.Models.GenerateContent(ctx, a.model, genai.Text(prompt), cfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	// Use the Text() helper method to get concatenated text from all parts
	text := result.Text()
	if text == "" {
		return "", fmt.Errorf("no text content in response")
	}
	return text, nil
}

// GroupErrors uses LLM to group similar errors.
func (a *DebugAgent) GroupErrors(ctx context.Context, errors []ErrorLog) ([]ErrorGroup, error) {
	if len(errors) == 0 {
		return nil, nil
	}

	// Limit errors for LLM processing to prevent context window issues
	errorsToProcess := errors
	if len(errors) > maxErrorsForGrouping {
		a.logger.Warn("Truncating errors for LLM grouping",
			zap.Int("original_count", len(errors)),
			zap.Int("truncated_count", maxErrorsForGrouping))
		errorsToProcess = errors[:maxErrorsForGrouping]
	}

	// Prepare error messages for the prompt
	var errorMessages []string
	for i, e := range errorsToProcess {
		errorMessages = append(errorMessages, fmt.Sprintf("%d. [%s] %s", i+1, e.Timestamp.Format(time.RFC3339), e.Message))
	}

	prompt := fmt.Sprintf(`You are an expert at analyzing error logs. Given the following error messages, group them by similarity (same root cause or pattern).

Error Messages:
%s

Respond with a JSON array of error groups. Each group should have:
- "pattern": A brief description of the error pattern
- "indices": An array of 1-based indices of errors belonging to this group

Example response:
[
  {"pattern": "Database connection timeout", "indices": [1, 3, 5]},
  {"pattern": "Authentication failure", "indices": [2, 4]}
]

Only respond with valid JSON, no other text.`, strings.Join(errorMessages, "\n"))

	result, err := a.generateContent(ctx, prompt, groupResponseSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to run LLM for grouping: %w", err)
	}

	// Parse the response
	var groupResponse []struct {
		Pattern string `json:"pattern"`
		Indices []int  `json:"indices"`
	}

	// Clean up response (remove markdown code blocks if present)
	responseText := extractJSON(result)

	if err := json.Unmarshal([]byte(responseText), &groupResponse); err != nil {
		a.logger.Error("Failed to parse grouping response",
			zap.Error(err),
			zap.String("response", responseText))
		// Fallback: treat all errors as one group
		return []ErrorGroup{{
			Pattern:        "Ungrouped errors",
			Representative: errorsToProcess[0],
			SimilarErrors:  errorsToProcess[1:],
			Count:          len(errorsToProcess),
		}}, nil
	}

	// Convert response to ErrorGroups
	var groups []ErrorGroup
	for _, g := range groupResponse {
		if len(g.Indices) == 0 {
			continue
		}

		group := ErrorGroup{
			Pattern: g.Pattern,
			Count:   len(g.Indices),
		}

		for i, idx := range g.Indices {
			if idx < 1 || idx > len(errorsToProcess) {
				continue
			}
			if i == 0 {
				group.Representative = errorsToProcess[idx-1]
			} else {
				group.SimilarErrors = append(group.SimilarErrors, errorsToProcess[idx-1])
			}
		}

		// Only add group if we have at least one valid error
		actualCount := 1 + len(group.SimilarErrors)
		if actualCount > 0 && group.Representative.Message != "" {
			group.Count = actualCount
			groups = append(groups, group)
		}
	}

	a.logger.Info("Grouped errors",
		zap.Int("error_count", len(errorsToProcess)),
		zap.Int("group_count", len(groups)))
	return groups, nil
}

// AnalyzeErrors uses LLM to analyze an error group with optional trace context.
func (a *DebugAgent) AnalyzeErrors(ctx context.Context, group ErrorGroup, traceLogs []string) (*ErrorAnalysis, error) {
	var traceContext string
	if len(traceLogs) > 0 {
		traceContext = fmt.Sprintf("\n\nTrace Context (related logs):\n%s", strings.Join(traceLogs, "\n"))
	}

	prompt := fmt.Sprintf(`You are an expert at diagnosing application errors. Analyze the following error group and provide actionable insights.

Error Pattern: %s
Error Count: %d
Representative Error: %s
%s

Respond with a JSON object containing:
- "summary": A brief summary of what's happening (1-2 sentences)
- "possible_causes": An array of 2-4 possible root causes
- "suggestions": An array of 2-4 actionable suggestions to fix or investigate

Only respond with valid JSON, no other text.`, group.Pattern, group.Count, group.Representative.Message, traceContext)

	result, err := a.generateContent(ctx, prompt, analysisResponseSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to run LLM for analysis: %w", err)
	}

	// Parse the response
	var analysis struct {
		Summary        string   `json:"summary"`
		PossibleCauses []string `json:"possible_causes"`
		Suggestions    []string `json:"suggestions"`
	}

	// Clean up response
	responseText := extractJSON(result)

	if err := json.Unmarshal([]byte(responseText), &analysis); err != nil {
		a.logger.Error("Failed to parse analysis response",
			zap.Error(err),
			zap.String("response", responseText))
		// Fallback with basic analysis
		return &ErrorAnalysis{
			Summary:        fmt.Sprintf("Error pattern: %s (%d occurrences)", group.Pattern, group.Count),
			PossibleCauses: []string{"Unable to determine root cause automatically"},
			Suggestions:    []string{"Review error logs manually", "Check application metrics"},
		}, nil
	}

	return &ErrorAnalysis{
		Summary:        analysis.Summary,
		PossibleCauses: analysis.PossibleCauses,
		Suggestions:    analysis.Suggestions,
	}, nil
}
