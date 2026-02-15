.PHONY: build clean run test

build:
	go build -o mastermind .

clean:
	rm -f mastermind

run: build
	./mastermind

test:
	@command -v gotestsum >/dev/null 2>&1 || go install gotest.tools/gotestsum@latest
	$(shell go env GOPATH)/bin/gotestsum --format testdox ./...
