BINARY := solaredge-exporter
VERSION := 0.1.0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint docker run clean

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

docker:
	docker build -t $(BINARY):$(VERSION) .

run: build
	./$(BINARY) $(ARGS)

clean:
	rm -f $(BINARY)
