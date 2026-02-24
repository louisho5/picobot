package channels

import (
	"os"
	"strings"
)

const allowPublicChannelsEnv = "PICOBOT_ALLOW_PUBLIC_CHANNELS"

func allowPublicChannels() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(allowPublicChannelsEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
