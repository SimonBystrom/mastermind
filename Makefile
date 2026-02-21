VERSION ?= dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY  := mastermind

.PHONY: build clean run test install uninstall snapshot

build:
	go build $(LDFLAGS) -o $(BINARY) .

clean:
	rm -f $(BINARY)
	rm -rf dist

run: build
	./$(BINARY)

test:
	@command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@latest
	$(shell go env GOPATH)/bin/gotestsum --format testdox ./...

install: build
	install -d /usr/local/bin
	install -m 755 $(BINARY) /usr/local/bin/$(BINARY)
	$(BINARY) --init-config

uninstall:
	rm -f /usr/local/bin/$(BINARY)

snapshot:
	goreleaser release --snapshot --clean
