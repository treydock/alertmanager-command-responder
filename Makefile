NOW := $(shell date +'%Y-%m-%d_%T')

GITSHA := $(shell git rev-parse HEAD)
GITVERSION := $(shell git describe --abbrev=0 --tags)

build:
	go build -ldflags "-X main.gitSha=${GITSHA} -X main.gitTag=${GITVERSION} -X main.buildTime=${NOW}" -o prometheus-alertmanager-responder
