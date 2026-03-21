package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/local/picobot/internal/cron"
)

// CronTool schedules delayed/recurring tasks via the cron scheduler.
// It holds a channel/chatID context (set per-incoming-message) so fired jobs
// know where to send their notification.
type CronTool struct {
	scheduler *cron.Scheduler
	channel   string
	chatID    string
}

func NewCronTool(scheduler *cron.Scheduler) *CronTool {
	return &CronTool{scheduler: scheduler}
}

func (t *CronTool) Name() string { return "cron" }
func (t *CronTool) Description() string {
	return "Schedule one-time or recurring reminders/tasks. Actions: add (schedule), list (show pending), cancel (remove by name)."
}

func (t *CronTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action: add (schedule a new job), list (show pending jobs), cancel (remove a job by name)",
				"enum":        []string{"add", "list", "cancel"},
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "A short name for the job (used to identify it for cancellation)",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The reminder message or task description to deliver when the job fires",
			},
			"delay": map[string]interface{}{
				"type":        "string",
				"description": "How long to wait before first firing, e.g. '2m', '1h30m', '30s', '1h'. Uses Go duration format.",
			},
			"recurring": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, the job will repeat at the specified interval. If false or omitted, fires only once.",
			},
			"interval": map[string]interface{}{
				"type":        "string",
				"description": "For recurring jobs: how often to repeat (minimum 2m). Uses Go duration format.",
			},
		},
		"required": []string{"action"},
	}
}

// SetContext sets the originating channel and chat for scheduled jobs.
func (t *CronTool) SetContext(channel, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

func (t *CronTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)

	switch action {
	case "add":
		name, _ := args["name"].(string)
		message, _ := args["message"].(string)
		delayStr, _ := args["delay"].(string)
		recurring, _ := args["recurring"].(bool)
		intervalStr, _ := args["interval"].(string)

		if name == "" {
			name = "reminder"
		}
		if message == "" {
			return "", fmt.Errorf("cron add: 'message' is required")
		}
		if delayStr == "" {
			return "", fmt.Errorf("cron add: 'delay' is required (e.g. '2m', '1h')")
		}

		delay, err := time.ParseDuration(delayStr)
		if err != nil {
			return "", fmt.Errorf("cron add: invalid delay %q: %v", delayStr, err)
		}
		if delay <= 0 {
			return "", fmt.Errorf("cron add: delay must be positive")
		}

		// Handle recurring jobs
		if recurring {
			if intervalStr == "" {
				intervalStr = delayStr // use delay as interval if not specified
			}
			interval, err := time.ParseDuration(intervalStr)
			if err != nil {
				return "", fmt.Errorf("cron add: invalid interval %q: %v", intervalStr, err)
			}
			// Enforce minimum 2-minute interval to prevent abuse
			if interval < 2*time.Minute {
				return "", fmt.Errorf("cron add: recurring interval must be at least 2m (got %v)", interval)
			}
			id := t.scheduler.AddRecurring(name, message, interval, t.channel, t.chatID)
			return fmt.Sprintf("Scheduled recurring job %q (id: %s). Will fire in %v, then repeat every %v.", name, id, delay, interval), nil
		}

		// One-time job
		id := t.scheduler.Add(name, message, delay, t.channel, t.chatID)
		return fmt.Sprintf("Scheduled job %q (id: %s). Will fire in %v.", name, id, delay), nil

	case "list":
		jobs := t.scheduler.List()
		if len(jobs) == 0 {
			return "No pending jobs.", nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d pending job(s):\n", len(jobs))
		for _, j := range jobs {
			remaining := time.Until(j.FireAt).Round(time.Second)
			fmt.Fprintf(&sb, "- %s (%s): %q — fires in %v\n", j.Name, j.ID, j.Message, remaining)
		}
		return sb.String(), nil

	case "cancel":
		name, _ := args["name"].(string)
		if name == "" {
			return "", fmt.Errorf("cron cancel: 'name' is required")
		}
		if t.scheduler.CancelByName(name) {
			return fmt.Sprintf("Cancelled job %q.", name), nil
		}
		return fmt.Sprintf("No job found with name %q.", name), nil

	default:
		return "", fmt.Errorf("cron: unknown action %q (use add, list, or cancel)", action)
	}
}
