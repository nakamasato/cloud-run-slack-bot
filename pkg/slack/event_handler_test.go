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
	}{
		{
			name: "test",
			m: &Memory{
				data: map[string]string{
					"key": "value",
				},
			},
			key: "key",
			val: "value2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Set(tt.key, tt.val)
			if got, _ := tt.m.Get(tt.key); got != tt.val {
				t.Errorf("Memory.Set() = %v, want %v", got, tt.val)
			}
		})
	}
}
