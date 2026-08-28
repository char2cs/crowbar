package permission_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestAutoApprove(t *testing.T) {
	cases := []struct {
		name  string
		level permission.Level
		risk  engineagents.RiskTier
		want  bool
	}{
		{"guarded holds a read-only tier", permission.Guarded, engineagents.RiskReadOnly, false},
		{"guarded holds a standard tier", permission.Guarded, engineagents.RiskStandard, false},
		{"guarded holds a sensitive tier", permission.Guarded, engineagents.RiskSensitive, false},
		{"guarded still auto-approves internal", permission.Guarded, engineagents.RiskInternal, true},

		{"trusted auto-approves read-only", permission.Trusted, engineagents.RiskReadOnly, true},
		{"trusted auto-approves standard", permission.Trusted, engineagents.RiskStandard, true},
		{"trusted still holds sensitive", permission.Trusted, engineagents.RiskSensitive, false},
		{"trusted auto-approves internal", permission.Trusted, engineagents.RiskInternal, true},

		{"full-auto auto-approves read-only", permission.FullAuto, engineagents.RiskReadOnly, true},
		{"full-auto auto-approves standard", permission.FullAuto, engineagents.RiskStandard, true},
		{"full-auto auto-approves sensitive", permission.FullAuto, engineagents.RiskSensitive, true},
		{"full-auto auto-approves internal", permission.FullAuto, engineagents.RiskInternal, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, permission.AutoApprove(tc.level, tc.risk))
		})
	}
}

func TestValid(t *testing.T) {
	cases := []struct {
		level permission.Level
		want  bool
	}{
		{permission.Guarded, true},
		{permission.Trusted, true},
		{permission.FullAuto, true},
		{permission.Level("yolo"), false},
		{permission.Level(""), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.level), func(t *testing.T) {
			assert.Equal(t, tc.want, permission.Valid(tc.level))
		})
	}
}
