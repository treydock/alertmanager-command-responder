NOW := $(shell date +'%Y-%m-%d_%T')

GITSHA := $(shell git rev-parse HEAD)
GITVERSION := $(shell git describe --abbrev=0 --tags)
INTERNAL := "github.com/treydock/alertmanager-command-responder/internal"

build:
	go build -ldflags "-X ${INTERNAL}.gitSha=${GITSHA} -X ${INTERNAL}.gitTag=${GITVERSION} -X ${INTERNAL}.buildTime=${NOW}" -o alertmanager-command-responder ./cmd/alertmanager-command-responder

docker-build:
	docker build -t treydock/alertmanager-command-responder:latest .
