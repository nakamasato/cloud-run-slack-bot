package slack

import (
	"testing"
)

func TestMemory_Get(t *testing.T) {
	tests := []struct {
		name string
		m    *Memory
		key  string
		want string
	}{
		{
			name: "test",
			m: &Memory{
				data: map[string]string{
					"key": "value",
				},
				resourceType: map[string]string{},
			},
			key:  "key",
			want: "value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := tt.m.Get(tt.key); got != tt.want {
				t.Errorf("Memory.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemory_Set(t *testing.T) {
	tests := []struct {
		name         string
		m            *Memory
		key          string
		val          string
		resourceType string
		expectIsJob  bool
	}{
		{
			name: "service",
			m: &Memory{
				data:         map[string]string{"key": "value"},
				resourceType: map[string]string{},
			},
			key:          "key",
			val:          "value2",
			resourceType: "service",
			expectIsJob:  false,
		},
		{
			name: "job",
			m: &Memory{
				data:         map[string]string{"key2": "value"},
				resourceType: map[string]string{},
			},
			key:          "key2",
			val:          "job1",
			resourceType: "job",
			expectIsJob:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Set(tt.key, tt.val, tt.resourceType)
			if got, _ := tt.m.Get(tt.key); got != tt.val {
				t.Errorf("Memory.Get() = %v, want %v", got, tt.val)
			}
			if got := tt.m.IsJob(tt.key); got != tt.expectIsJob {
				t.Errorf("Memory.IsJob() = %v, want %v", got, tt.expectIsJob)
			}
			if got := tt.m.GetResourceType(tt.key); got != tt.resourceType {
				t.Errorf("Memory.GetResourceType() = %v, want %v", got, tt.resourceType)
			}
		})
	}
}

func TestMemory_IsJob(t *testing.T) {
	tests := []struct {
		name string
		m    *Memory
		key  string
		want bool
	}{
		{
			name: "is job",
			m: &Memory{
				data:         map[string]string{"key": "value"},
				resourceType: map[string]string{"key": "job"},
			},
			key:  "key",
			want: true,
		},
		{
			name: "is service",
			m: &Memory{
				data:         map[string]string{"key": "value"},
				resourceType: map[string]string{"key": "service"},
			},
			key:  "key",
			want: false,
		},
		{
			name: "key not found",
			m: &Memory{
				data:         map[string]string{},
				resourceType: map[string]string{},
			},
			key:  "nonexistent",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.IsJob(tt.key); got != tt.want {
				t.Errorf("Memory.IsJob() = %v, want %v", got, tt.want)
			}
		})
	}
}
