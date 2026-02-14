.PHONY: build clean run

build:
	go build -o mastermind .

clean:
	rm -f mastermind

run: build
	./mastermind
