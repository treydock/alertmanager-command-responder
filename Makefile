DOCKER_ARCHS ?= amd64 armv7 arm64 ppc64le s390x
DOCKER_REPO	 ?= treydock

include Makefile.common

DOCKER_IMAGE_NAME ?= alertmanager-command-responder

coverage:
	go test -race -coverpkg=./... -coverprofile=coverage.txt -covermode=atomic ./...
