//go:build lite

package channels

import (
	"context"
	"fmt"
	"log"

	"github.com/local/picobot/internal/chat"
)

// StartWhatsApp is a no-op stub used when the binary is built with the
// 'lite' build tag. If WhatsApp is enabled in the config it logs a clear
// warning and returns nil so the gateway continues with other channels.
func StartWhatsApp(ctx context.Context, hub *chat.Hub, dbPath string, allowFrom []string) error {
	log.Println("whatsapp: channel not available in 'lite' version.")
	return nil
}

// SetupWhatsApp returns an error explaining how to build with WhatsApp support.
func SetupWhatsApp(dbPath string) error {
	return fmt.Errorf("WhatsApp support is not compiled into this binary\n" +
		"Download the full version of picobot from the github releases page")
}
