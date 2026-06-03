package resolver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/resolver"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

func TestResolve(
	t *testing.T,
) {
	cfg := config.IntelligenceConfig{
		Low:    "claude-haiku-4-5-20251001",
		Medium: "claude-sonnet-4-6",
		High:   "claude-opus-4-7",
	}

	cases := []struct {
		name        string
		level       flow.IntelligenceLevel
		wantBin     string
		wantArgs    []string
		wantErr     bool
	}{
		{
			name:     "low maps to claude-code-acp",
			level:    flow.IntelligenceLow,
			wantBin:  "claude-code-acp",
			wantArgs: nil,
		},
		{
			name:     "medium maps to claude-code-acp",
			level:    flow.IntelligenceMedium,
			wantBin:  "claude-code-acp",
			wantArgs: nil,
		},
		{
			name:     "high maps to claude-code-acp",
			level:    flow.IntelligenceHigh,
			wantBin:  "claude-code-acp",
			wantArgs: nil,
		},
		{
			name:    "unknown intelligence level returns error",
			level:   flow.IntelligenceLevel("unknown"),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, args, err := resolver.Resolve(tc.level, cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantBin, bin)
			assert.Equal(t, tc.wantArgs, args)
		})
	}
}

func TestResolve_unknownModelPrefix(
	t *testing.T,
) {
	cfg := config.IntelligenceConfig{
		Low: "openai-gpt-4o",
	}
	_, _, err := resolver.Resolve(flow.IntelligenceLow, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown model prefix")
}
