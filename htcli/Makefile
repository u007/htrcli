.PHONY: build run test clean install

build:
	go build -o bin/htcli ./cmd/htcli

run:
	go run ./cmd/htcli

test:
	go test ./...

clean:
	rm -rf bin/

install:
	go install ./cmd/htcli
