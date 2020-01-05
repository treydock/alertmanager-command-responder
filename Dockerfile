FROM golang:1.13 AS builder
RUN mkdir /build
ADD . /build/
WORKDIR /build
RUN CGO_ENABLED=0 make build

FROM alpine
WORKDIR /
COPY --from=builder /build/alertmanager-command-responder /alertmanager-command-responder
ENTRYPOINT ["/alertmanager-command-responder"]
