PROMU := $(shell go env GOPATH)/bin/promu

PREFIX ?= $(shell pwd)

build: promu
	@$(PROMU) build --verbose --prefix $(PREFIX)

promu:
	go get -u github.com/prometheus/promu

docker-build:
	docker build -t treydock/alertmanager-command-responder:latest .
