package adk

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMergeGroupsByTrace(t *testing.T) {
	tests := []struct {
		name           string
		groups         []ErrorGroup
		expectedGroups int
		description    string
	}{
		{
			name: "strong overlap - merge two groups",
			groups: []ErrorGroup{
				{
					Pattern: "Database timeout",
					Representative: ErrorLog{
						Message:   "connection timeout",
						Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error2", Timestamp: time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC), TraceID: "T2"},
					},
					Count: 2,
				},
				{
					Pattern: "Network error",
					Representative: ErrorLog{
						Message:   "network unreachable",
						Timestamp: time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error4", Timestamp: time.Date(2024, 1, 1, 10, 3, 0, 0, time.UTC), TraceID: "T2"},
					},
					Count: 2,
				},
			},
			expectedGroups: 1,
			description:    "Groups sharing 100% of traces should merge",
		},
		{
			name: "no overlap - no merge",
			groups: []ErrorGroup{
				{
					Pattern: "Auth failure",
					Representative: ErrorLog{
						Message:   "auth failed",
						Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error2", Timestamp: time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC), TraceID: "T2"},
						{Message: "error3", Timestamp: time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC), TraceID: "T3"},
					},
					Count: 3,
				},
				{
					Pattern: "Validation error",
					Representative: ErrorLog{
						Message:   "invalid input",
						Timestamp: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
						TraceID:   "T4",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error5", Timestamp: time.Date(2024, 1, 1, 10, 6, 0, 0, time.UTC), TraceID: "T5"},
					},
					Count: 2,
				},
			},
			expectedGroups: 2,
			description:    "Groups with no shared traces should not merge",
		},
		{
			name: "no traces - no merge",
			groups: []ErrorGroup{
				{
					Pattern: "Error A",
					Representative: ErrorLog{
						Message:   "error A",
						Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						TraceID:   "",
					},
					SimilarErrors: []ErrorLog{},
					Count:         1,
				},
				{
					Pattern: "Error B",
					Representative: ErrorLog{
						Message:   "error B",
						Timestamp: time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC),
						TraceID:   "",
					},
					SimilarErrors: []ErrorLog{},
					Count:         1,
				},
			},
			expectedGroups: 2,
			description:    "Groups without traces should remain separate",
		},
		{
			name: "transitive merge",
			groups: []ErrorGroup{
				{
					Pattern: "Error A",
					Representative: ErrorLog{
						Message:   "error A",
						Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error A2", Timestamp: time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC), TraceID: "T2"},
					},
					Count: 2,
				},
				{
					Pattern: "Error B",
					Representative: ErrorLog{
						Message:   "error B",
						Timestamp: time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC),
						TraceID:   "T2",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error B2", Timestamp: time.Date(2024, 1, 1, 10, 3, 0, 0, time.UTC), TraceID: "T3"},
					},
					Count: 2,
				},
				{
					Pattern: "Error C",
					Representative: ErrorLog{
						Message:   "error C",
						Timestamp: time.Date(2024, 1, 1, 10, 4, 0, 0, time.UTC),
						TraceID:   "T3",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error C2", Timestamp: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC), TraceID: "T4"},
					},
					Count: 2,
				},
			},
			expectedGroups: 1,
			description:    "Transitively related groups (A→B→C) should all merge",
		},
		{
			name: "single group - return as-is",
			groups: []ErrorGroup{
				{
					Pattern: "Single error",
					Representative: ErrorLog{
						Message:   "single error",
						Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{},
					Count:         1,
				},
			},
			expectedGroups: 1,
			description:    "Single group should be returned as-is",
		},
		{
			name: "asymmetric overlap - merge when one direction exceeds threshold",
			groups: []ErrorGroup{
				{
					Pattern: "Small group",
					Representative: ErrorLog{
						Message:   "error small",
						Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error small2", Timestamp: time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC), TraceID: "T2"},
					},
					Count: 2,
				},
				{
					Pattern: "Large group",
					Representative: ErrorLog{
						Message:   "error large",
						Timestamp: time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC),
						TraceID:   "T1",
					},
					SimilarErrors: []ErrorLog{
						{Message: "error2", Timestamp: time.Date(2024, 1, 1, 10, 3, 0, 0, time.UTC), TraceID: "T3"},
						{Message: "error3", Timestamp: time.Date(2024, 1, 1, 10, 4, 0, 0, time.UTC), TraceID: "T4"},
						{Message: "error4", Timestamp: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC), TraceID: "T5"},
					},
					Count: 4,
				},
			},
			expectedGroups: 1,
			description:    "Should merge when small group has >50% overlap (even if large group has <50%)",
		},
		{
			name:           "empty groups",
			groups:         []ErrorGroup{},
			expectedGroups: 0,
			description:    "Empty input should return empty output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &DebugAgent{logger: zap.NewNop()}
			result := agent.MergeGroupsByTrace(tt.groups)

			if len(result) != tt.expectedGroups {
				t.Errorf("%s: expected %d groups, got %d", tt.description, tt.expectedGroups, len(result))
			}

			// Verify total error count is preserved
			originalTotal := 0
			for _, g := range tt.groups {
				originalTotal += g.Count
			}
			resultTotal := 0
			for _, g := range result {
				resultTotal += g.Count
			}
			if originalTotal != resultTotal {
				t.Errorf("Total error count mismatch: original=%d, result=%d", originalTotal, resultTotal)
			}
		})
	}
}

func TestMergeGroupsByTrace_PatternMerging(t *testing.T) {
	groups := []ErrorGroup{
		{
			Pattern: "Pattern A",
			Representative: ErrorLog{
				Message:   "error A",
				Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				TraceID:   "T1",
			},
			Count: 1,
		},
		{
			Pattern: "Pattern B",
			Representative: ErrorLog{
				Message:   "error B",
				Timestamp: time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC),
				TraceID:   "T1",
			},
			Count: 1,
		},
	}

	agent := &DebugAgent{logger: zap.NewNop()}
	result := agent.MergeGroupsByTrace(groups)

	if len(result) != 1 {
		t.Fatalf("Expected 1 merged group, got %d", len(result))
	}

	// Pattern should be merged with " / " separator
	if result[0].Pattern != "Pattern A / Pattern B" && result[0].Pattern != "Pattern B / Pattern A" {
		t.Errorf("Expected merged pattern 'Pattern A / Pattern B' or 'Pattern B / Pattern A', got '%s'", result[0].Pattern)
	}

	// Count should be sum of both groups
	if result[0].Count != 2 {
		t.Errorf("Expected merged count 2, got %d", result[0].Count)
	}
}

func TestMergeGroupsByTrace_EarliestRepresentative(t *testing.T) {
	groups := []ErrorGroup{
		{
			Pattern: "Pattern A",
			Representative: ErrorLog{
				Message:   "later error",
				Timestamp: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
				TraceID:   "T1",
			},
			Count: 1,
		},
		{
			Pattern: "Pattern B",
			Representative: ErrorLog{
				Message:   "earlier error",
				Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				TraceID:   "T1",
			},
			Count: 1,
		},
	}

	agent := &DebugAgent{logger: zap.NewNop()}
	result := agent.MergeGroupsByTrace(groups)

	if len(result) != 1 {
		t.Fatalf("Expected 1 merged group, got %d", len(result))
	}

	// Representative should be the earliest error
	if result[0].Representative.Message != "earlier error" {
		t.Errorf("Expected representative to be 'earlier error', got '%s'", result[0].Representative.Message)
	}

	// Later error should be in SimilarErrors
	if len(result[0].SimilarErrors) != 1 {
		t.Errorf("Expected 1 similar error, got %d", len(result[0].SimilarErrors))
	}
	if len(result[0].SimilarErrors) > 0 && result[0].SimilarErrors[0].Message != "later error" {
		t.Errorf("Expected similar error to be 'later error', got '%s'", result[0].SimilarErrors[0].Message)
	}
}

func TestUnionFind(t *testing.T) {
	uf := newUnionFind(5)

	// Initially, each element is its own parent
	for i := 0; i < 5; i++ {
		if uf.find(i) != i {
			t.Errorf("Expected find(%d) = %d, got %d", i, i, uf.find(i))
		}
	}

	// Union 0 and 1
	uf.union(0, 1)
	if uf.find(0) != uf.find(1) {
		t.Errorf("Expected 0 and 1 to have same root after union")
	}

	// Union 2 and 3
	uf.union(2, 3)
	if uf.find(2) != uf.find(3) {
		t.Errorf("Expected 2 and 3 to have same root after union")
	}

	// 0 and 2 should still be separate
	if uf.find(0) == uf.find(2) {
		t.Errorf("Expected 0 and 2 to have different roots")
	}

	// Union 1 and 2 (transitively merges {0,1} and {2,3})
	uf.union(1, 2)
	if uf.find(0) != uf.find(3) {
		t.Errorf("Expected 0 and 3 to have same root after transitive union")
	}

	// 4 should still be separate
	if uf.find(4) == uf.find(0) {
		t.Errorf("Expected 4 to be separate from merged group")
	}
}
