---
name: cron
description: Schedule one-time reminders and recurring tasks
---

# Cron

Use the `cron` tool to schedule one-time reminders or recurring tasks.

## Actions

- `add` — schedule a new one-time or recurring job
- `list` — show all pending jobs
- `cancel` — remove a job by name

## Examples

### One-time Reminders

Set a one-time reminder:

```
cron(action="add", name="break-reminder", message="Time to take a break!", delay="20m")
```

Longer delay:

```
cron(action="add", name="standup", message="Daily standup in 5 minutes", delay="1h")
```

### Recurring Tasks

**Important:** Recurring jobs have a **minimum interval of 2 minutes** to prevent abuse.

Daily morning reminder (every 24 hours):

```
cron(action="add", name="morning-standup", message="Good morning! Time for standup", delay="1h", recurring=true, interval="24h")
```

Every hour check:

```
cron(action="add", name="hourly-reminder", message="Hourly check-in", delay="1h", recurring=true, interval="1h")
```

Every 30 minutes:

```
cron(action="add", name="water-reminder", message="Drink water!", delay="30m", recurring=true, interval="30m")
```

### Manage Jobs

List all pending jobs:

```
cron(action="list")
```

Cancel a job:

```
cron(action="cancel", name="break-reminder")
```

## Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `action` | string | Yes | `add`, `list`, or `cancel` |
| `name` | string | No | Job name (default: "reminder") |
| `message` | string | Yes (for add) | The reminder message |
| `delay` | string | Yes (for add) | Initial delay before first firing |
| `recurring` | boolean | No | If true, repeats at interval |
| `interval` | string | No | Repeat interval (min: 2m). Defaults to `delay` if not specified. |

## Duration Format

Use Go duration strings:

| User says | Duration value |
|---|---|
| 2 minutes | `2m` |
| 30 minutes | `30m` |
| 1 hour | `1h` |
| 1 hour 30 minutes | `1h30m` |
| 30 seconds | `30s` |
| 1 day | `24h` |
| 1 week | `168h` |

## Notes

- One-time jobs are removed after firing
- Recurring jobs continue until cancelled
- Minimum recurring interval: **2 minutes**
- Jobs persist only while the gateway is running (not saved to disk)
