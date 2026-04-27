.PHONY: darkish
darkish:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w" -o bin/darkish ./cmd/darkish
