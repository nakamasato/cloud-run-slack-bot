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
				isJob: map[string]bool{},
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
		name string
		m    *Memory
		key  string
		val  string
		isJob bool
	}{
		{
			name: "service",
			m: &Memory{
				data:  map[string]string{"key": "value"},
				isJob: map[string]bool{},
			},
			key:   "key",
			val:   "value2",
			isJob: false,
		},
		{
			name: "job",
			m: &Memory{
				data:  map[string]string{"key2": "value"},
				isJob: map[string]bool{},
			},
			key:   "key2",
			val:   "job1",
			isJob: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Set(tt.key, tt.val, tt.isJob)
			if got, _ := tt.m.Get(tt.key); got != tt.val {
				t.Errorf("Memory.Get() = %v, want %v", got, tt.val)
			}
			if got := tt.m.IsJob(tt.key); got != tt.isJob {
				t.Errorf("Memory.IsJob() = %v, want %v", got, tt.isJob)
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
				data: map[string]string{"key": "value"},
				isJob: map[string]bool{"key": true},
			},
			key:  "key",
			want: true,
		},
		{
			name: "is service",
			m: &Memory{
				data: map[string]string{"key": "value"},
				isJob: map[string]bool{"key": false},
			},
			key:  "key",
			want: false,
		},
		{
			name: "key not found",
			m: &Memory{
				data: map[string]string{},
				isJob: map[string]bool{},
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
