PROMU := $(shell go env GOPATH)/bin/promu

GITSHA := $(shell git rev-parse HEAD)
GITVERSION := $(shell git describe --abbrev=0 --tags)

build:
#	go build -ldflags "-X main.gitSha=${GITSHA} -X main.gitTag=${GITVERSION} -X main.buildTime=${NOW}" -o alertmanager-command-responder ./cmd/alertmanager-command-responder
	@$(PROMU) build --verbose

promu:
	go get -u github.com/prometheus/promu

docker-build:
	docker build -t treydock/alertmanager-command-responder:latest .
