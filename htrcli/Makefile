.PHONY: build run test clean install

build:
	go build -o bin/htrcli ./cmd/htrcli

run:
	go run ./cmd/htrcli

test:
	go test ./...

clean:
	rm -rf bin/

install:
	go install ./cmd/htrcli
