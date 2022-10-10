ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:glibc
LABEL maintainer="Trey Dockendorf <treydock@gmail.com>"
ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/alertmanager-command-responder /alertmanager-command-responder
EXPOSE 10000
ENTRYPOINT ["/alertmanager-command-responder"]
