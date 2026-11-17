.PHONY: build web windows test check third-party

VERSION ?= dev

web:
	npm --prefix web run build

build: web
	go build -ldflags "-X main.version=$(VERSION)" -o dist/lanbay ./cmd/lanbay

windows: web
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o dist/lanbay.exe ./cmd/lanbay

test:
	CGO_ENABLED=0 go test -count=1 -race ./...

third-party:
	bash scripts/gen-third-party.sh

check: third-party
	gofmt -l .
	go vet ./...
	bash scripts/check-spdx.sh
