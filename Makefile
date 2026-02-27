.PHONY: build, clean

build: linux_amd64 linux_arm64 mac_arm64 linux_amd64_lite linux_arm64_lite mac_arm64_lite
		@echo "All builds completed."

linux_amd64:
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_linux_amd64 ./cmd/picobot

linux_arm64:
		GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_linux_arm64 ./cmd/picobot

mac_arm64:
		GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_mac_arm64 ./cmd/picobot

linux_amd64_lite:
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -tags lite -o picobot_linux_amd64_lite ./cmd/picobot

linux_arm64_lite:
		GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -tags lite -o picobot_linux_arm64_lite ./cmd/picobot

mac_arm64_lite:
		GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -tags lite -o picobot_mac_arm64_lite ./cmd/picobot

clean:
		rm -f picobot_linux_amd64 picobot_linux_arm64 picobot_mac_arm64 picobot_linux_amd64_lite picobot_linux_arm64_lite picobot_mac_arm64_lite