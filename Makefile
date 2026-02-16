VERSION ?= dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY  := mastermind
TARGETS := darwin-arm64 darwin-amd64 linux-arm64 linux-amd64

.PHONY: build clean run test install uninstall release

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

release: clean
	@mkdir -p dist
	@for target in $(TARGETS); do \
		os=$${target%%-*}; \
		arch=$${target##*-}; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o dist/$(BINARY) . && \
		tar -czf dist/$(BINARY)-$(VERSION)-$$os-$$arch.tar.gz -C dist $(BINARY) && \
		rm -f dist/$(BINARY); \
	done
	@echo "Release artifacts in dist/"
