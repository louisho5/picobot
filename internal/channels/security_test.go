package channels

import "testing"

func TestAllowPublicChannelsEnv(t *testing.T) {
	t.Setenv(allowPublicChannelsEnv, "1")
	if !allowPublicChannels() {
		t.Fatalf("expected allowPublicChannels to be true when env is set")
	}
}

func TestAllowPublicChannelsDefaultFalse(t *testing.T) {
	t.Setenv(allowPublicChannelsEnv, "")
	if allowPublicChannels() {
		t.Fatalf("expected allowPublicChannels to be false by default")
	}
}
