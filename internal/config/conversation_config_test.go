package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConversationConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{name: "nil defaults to false", enabled: nil, want: false},
		{name: "explicit true", enabled: boolPtr(true), want: true},
		{name: "explicit false", enabled: boolPtr(false), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ConversationConfig{Enabled: tt.enabled}
			assert.Equal(t, tt.want, c.IsEnabled())
		})
	}
}

func boolPtr(v bool) *bool { return &v }
