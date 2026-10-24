package config

import (
	"testing"
	"time"
)

func TestAgentTimeout(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{"default for zero", 0, time.Duration(DefaultAgentTimeoutS) * time.Second},
		{"default for negative", -1, time.Duration(DefaultAgentTimeoutS) * time.Second},
		{"explicit value", 45, 45 * time.Second},
		{"five minutes", 300, 5 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AgentTimeout(tc.seconds); got != tc.want {
				t.Errorf("AgentTimeout(%d) = %s, want %s", tc.seconds, got, tc.want)
			}
		})
	}
}

func TestApplyEnvOverrides_AgentTimeoutS(t *testing.T) {
	cfg := DefaultConfig()
	t.Setenv("PICOBOT_AGENT_TIMEOUT_S", "120")
	applyEnvOverrides(&cfg)
	if cfg.Agents.Defaults.AgentTimeoutS != 120 {
		t.Errorf("AgentTimeoutS = %d, want 120", cfg.Agents.Defaults.AgentTimeoutS)
	}
}

func TestApplyEnvOverrides_AgentTimeoutS_IgnoresInvalid(t *testing.T) {
	cfg := DefaultConfig()
	want := cfg.Agents.Defaults.AgentTimeoutS
	t.Setenv("PICOBOT_AGENT_TIMEOUT_S", "nope")
	applyEnvOverrides(&cfg)
	if cfg.Agents.Defaults.AgentTimeoutS != want {
		t.Errorf("AgentTimeoutS = %d, want unchanged %d", cfg.Agents.Defaults.AgentTimeoutS, want)
	}
}

func TestHistoryDefaultsAndEnvOverrides(t *testing.T) {
	if HistoryRetention(0) != time.Duration(DefaultHistoryRetentionDays)*24*time.Hour {
		t.Errorf("unexpected default retention %s", HistoryRetention(0))
	}
	if HistoryContextChars(-1) != DefaultHistoryContextChars || HistoryContextChars(500) != 500 {
		t.Errorf("unexpected HistoryContextChars fallback")
	}

	cfg := DefaultConfig()
	t.Setenv("PICOBOT_HISTORY_RETENTION_DAYS", "21")
	t.Setenv("PICOBOT_HISTORY_CONTEXT_CHARS", "50000")
	applyEnvOverrides(&cfg)
	if cfg.Agents.Defaults.HistoryRetentionDays != 21 || cfg.Agents.Defaults.HistoryContextChars != 50000 {
		t.Errorf("env overrides not applied: %+v", cfg.Agents.Defaults)
	}
}
