BIN := dist/isphub-print-agent
VERSION ?= 0.1.0

.PHONY: build test fmt

build:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
	  -ldflags "-s -w -X main.version=$(VERSION)" \
	  -o $(BIN) ./cmd/isphub-print-agent

test:
	go test ./...

fmt:
	gofmt -w cmd internal
