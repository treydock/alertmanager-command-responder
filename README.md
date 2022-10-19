# alertmanager-command-responder

[![Build Status](https://circleci.com/gh/treydock/alertmanager-command-responder/tree/main.svg?style=shield)](https://circleci.com/gh/treydock/alertmanager-command-responder)
[![GitHub release](https://img.shields.io/github/v/release/treydock/alertmanager-command-responder?include_prereleases&sort=semver)](https://github.com/treydock/alertmanager-command-responder/releases/latest)
![GitHub All Releases](https://img.shields.io/github/downloads/treydock/alertmanager-command-responder/total)
[![Go Report Card](https://goreportcard.com/badge/github.com/treydock/alertmanager-command-responder)](https://goreportcard.com/report/github.com/treydock/alertmanager-command-responder)
[![codecov](https://codecov.io/gh/treydock/alertmanager-command-responder/branch/main/graph/badge.svg)](https://codecov.io/gh/treydock/alertmanager-command-responder)

# Alertmanager Command Responder

The alertmanager-command-responder tool is a webhook for Alertmanager that can execute both local and remote commands when Alertmanager alerts are firing.

At this time both local and SSH based commands are supported.  The configuration of what to execute is handled by the alert annotations.

## Annotations

The following are the annotations a Prometheus alert can configure to modify the behavior of alertmanager-command-responder.

Annotation | Description | Default
-----|-------------|--------
`cr_status` | The Alert status to act on, `firing` or `resolved` | `firing`
`cr_ssh_user` | User for remote command execution | `ssh_user` value in configuration file or user running this service
`cr_ssh_key` | SSH private key for authentication for SSH command | `ssh_key` value in configuration file
`cr_ssh_cert` | SSH certificate for cert based authentication | `ssh_certificate` value in configuration file
`cr_ssh_host` | SSH remote host to run command | **required**
`cr_ssh_conn_timeout` | SSH connection timeout duration, eg: `5s` | `ssh_connection_timeout` value in configuration file or `5s`
`cr_ssh_cmd` | SSH command to execute on remote host | **optional**
`cr_ssh_cmd_timeout` | Duration for SSH command timeout, eg: `5s` | `ssh_command_timeout` value in configuration file or `10s`
`cr_local_cmd` | Local command to execute | **optional**
`cr_local_cmd_timeout` | Local command timeout duration, eg: `5s` | `local_command_timeout` value in configuration file or `10s`

## Configuration

Path to the YAML configuration is set using the `--config.file` flag.

Configuration options:

* `ssh_user` - The default username for the SSH connections. Can be overriden by annotations
* `ssh_password` - The password for the SSH connection, required if `ssh_private_key` is not specified
* `ssh_key` - The SSH private key for the SSH connection, required if `password` is not specified. Can be overriden by annotations
* `ssh_certificate` - The SSH certificate for the private key for the SSH connection
* `ssh_known_hosts` - Optional SSH known hosts file to use to verify hosts
* `ssh_host_key_algorithms` - Optional list of SSH host key algorithms to use
  * See constants beginning with `KeyAlgo*` in [crypto/ssh](https://godoc.org/golang.org/x/crypto/ssh#pkg-constants)
* `ssh_connection_timeout` - Optional timeout of the SSH connection, default `5s`.
* `ssh_command_timeout` - Default SSH command timeout, default `10s`. Can be overriden by annotations
* `local_command_timeout` - Default local command timeout, default `10s`. Can be overriden by annotations

## Install

Download the [latest release](https://github.com/treydock/alertmanager-command-responder/releases)

Add the user that will run the service.

```
groupadd -r cruser
useradd -r -d /var/lib/cruser -s /sbin/nologin -M -g cruser -M cruser
```

Install compiled binaries after extracting tar.gz from release page.

```
cp /tmp/alertmanager-command-responder /usr/local/bin/alertmanager-command-responder
```

Add systemd unit file and start service. Modify the `ExecStart` with desired flags.

```
cp systemd/alertmanager-command-responder.service /etc/systemd/system/alertmanager-command-responder.service
systemctl daemon-reload
systemctl start alertmanager-command-responder
```

## Build from source

To produce the `alertmanager-command-responder` binary:

```
make build
```

Or

```
go get github.com/treydock/alertmanager-command-responder/cmd/alertmanager-command-responder
```

## Alertmanager Configuration

The following is an example of adding the alertmanager-command-responder webhook to Alertmanager

```yaml
receivers:
  - name: command-responder
    webhook_configs:
      - url: http://localhost:10000/alerts
```

In your routes for alertmanager, add something like the following:

```yaml
route:
  receiver: ...
  group_by: ...
  routes:
    - receiver: command-responder
      repeat_interval: 4h
      continue: true
```
