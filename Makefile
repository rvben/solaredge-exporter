BINARY := solaredge-exporter
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint docker run clean release-patch release-minor release-major

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY):$(VERSION) .

run: build
	./$(BINARY) $(ARGS)

clean:
	rm -f $(BINARY)

release-patch:
	vership bump patch

release-minor:
	vership bump minor

release-major:
	vership bump major
