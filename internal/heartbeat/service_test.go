package heartbeat

import "testing"

func TestHasActiveTasks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "Default template with comments",
			content: `# Heartbeat

This file is checked periodically (every 60 seconds). Add tasks here that should run on a schedule.

## IMPORTANT RULES FOR HEARTBEAT PROCESSING

- After reviewing this file, take actions ONLY if there are explicit tasks listed below
- If there are no tasks (or all tasks are complete), do NOTHING — do not send any message, do not call write_memory or any memory tool
- NEVER log "heartbeat check complete", "system status: healthy", or any status message to memory files — these clutter memory with useless noise
- Heartbeat results are ephemeral: process, act if needed, then stop silently

## Periodic Tasks

<!-- Add tasks below. The agent will process them on each heartbeat check. -->
<!-- Example:
- Check server status at https://example.com/health
- Summarize unread messages
-->
`,
			want: false,
		},
		{
			name: "Template with active task under Periodic Tasks",
			content: `# Heartbeat

## Periodic Tasks

- Check CPU usage on prod
<!-- Example:
- Check server status at https://example.com/health
-->
`,
			want: true,
		},
		{
			name: "Template with active task under Tasks",
			content: `# Heartbeat

## Tasks

* Clean up temp files
`,
			want: true,
		},
		{
			name: "No headers but has active tasks",
			content: `
- Check mail status
`,
			want: true,
		},
		{
			name: "No headers and only template sentences",
			content: `
Never log "heartbeat check complete", "system status: healthy"
Heartbeat results are ephemeral
`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasActiveTasks(tt.content)
			if got != tt.want {
				t.Errorf("hasActiveTasks() = %v, want %v", got, tt.want)
			}
		})
	}
}
