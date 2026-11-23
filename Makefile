.PHONY: build web windows test check third-party ally-kit

VERSION ?= dev

web:
	npm --prefix web run build

build: web
	go build -ldflags "-X main.version=$(VERSION)" -o dist/lanbay ./cmd/lanbay

windows: web
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o dist/lanbay.exe ./cmd/lanbay

ally-kit:
	rm -rf dist/ally-kit dist/lanbay-m0-kit.zip dist/lanbay-m0-kit.zip.sha256
	mkdir -p dist/ally-kit/{volumes,winget-backend,recycle-bin,execution-context}
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/ally-kit/volumes/probe.exe ./spikes/volumes
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/ally-kit/winget-backend/probe.exe ./spikes/winget-backend
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/ally-kit/recycle-bin/probe.exe ./spikes/recycle-bin
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/ally-kit/execution-context/runner.exe ./spikes/execution-context
	cp spikes/volumes/README.md dist/ally-kit/volumes/
	cp spikes/winget-backend/{README.md,run.ps1,measure-module.ps1} dist/ally-kit/winget-backend/
	cp spikes/recycle-bin/{README.md,run.ps1,recycle.ps1} dist/ally-kit/recycle-bin/
	cp spikes/execution-context/{README.md,register-task.ps1,cleanup.ps1} dist/ally-kit/execution-context/
	cp spikes/bootstrap.ps1 dist/ally-kit/bootstrap.ps1
	go run ./tools/kitpack -source dist/ally-kit -output dist/lanbay-m0-kit.zip -sha256 dist/lanbay-m0-kit.zip.sha256

test:
	CGO_ENABLED=0 go test -count=1 -race ./...

third-party:
	bash scripts/gen-third-party.sh

check: third-party
	gofmt -l .
	go vet ./...
	bash scripts/check-spdx.sh

release: web
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o dist/lanbay.exe ./cmd/lanbay
	cd dist && (command -v shasum >/dev/null 2>&1 && shasum -a 256 lanbay.exe || sha256sum lanbay.exe) > lanbay.exe.sha256
