// Copyright 2022 Trey Dockendorf
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alert

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/treydock/alertmanager-command-responder/internal/config"
	"github.com/treydock/alertmanager-command-responder/internal/metrics"
	"github.com/treydock/alertmanager-command-responder/internal/utils"
)

const (
	statusAnnotation       = "cr_status"
	sshUserAnnotation      = "cr_ssh_user"
	sshKeyAnnotation       = "cr_ssh_key"
	sshCertAnnotation      = "cr_ssh_cert"
	sshHostAnnotation      = "cr_ssh_host"
	sshConnTimeout         = "cr_ssh_conn_timeout"
	sshCommandAnnotation   = "cr_ssh_cmd"
	sshCommandTimeout      = "cr_ssh_cmd_timeout"
	localCommandAnnotation = "cr_local_cmd"
	localCommandTimeout    = "cr_local_cmd_timeout"
)

type Alert struct {
	template.Alert
	logger   *slog.Logger
	Response AlertResponse `json:"response"`
}

type AlertResponse struct {
	Status               []string      `json:"status"`
	SSHUser              string        `json:"ssh_user"`
	SSHKey               string        `json:"ssh_key"`
	SSHCertificate       string        `json:"ssh_certificate"`
	SSHPassword          string        `json:"ssh_password"`
	SSHKnownHosts        string        `json:"ssh_known_hosts"`
	SSHHostKeyAlgorithms []string      `json:"ssh_host_key_algorithms"`
	SSHConnectionTimeout time.Duration `json:"ssh_connection_timeout"`
	SSHCommandTimeout    time.Duration `json:"ssh_command_timeout"`
	SSHHost              string        `json:"ssh_host"`
	SSHCommand           string        `json:"ssh_command"`
	LocalCommand         string        `json:"local_command"`
	LocalCommandTimeout  time.Duration `json:"local_command_timeout"`
}

func (a *Alert) Name() string {
	if val, ok := a.Labels["alertname"]; ok {
		return val
	}
	return a.Fingerprint
}

func (a *Alert) HandleAlert(c *config.Config, logger *slog.Logger) error {
	var err error
	a.logger = logger.With("alert", a.Fingerprint, "alertname", a.Name())
	a.logger.Debug("Handling alert")
	r, err := a.buildResponse(c)
	if err != nil {
		a.logger.Error("Error building alert response", "err", err)
		metrics.ErrorsTotal.Inc()
		return err
	}
	if !utils.SliceContains(r.Status, a.Status) {
		a.logger.Debug("Alert status does not match alert", "status", a.Status, "expected", strings.Join(r.Status, ","))
		return nil
	}
	a.Response = r

	if a.Response.LocalCommand == "" && a.Response.SSHCommand == "" {
		a.logger.Debug("ignore alert without local or SSH command")
	}
	start := time.Now()
	if a.Response.LocalCommand != "" {
		localLogger := a.logger.With("type", "local", "command", r.LocalCommand)
		err = a.Response.runLocalCommand(localLogger)
		if err != nil {
			localLogger.Error("Failed to run local command", "err", err)
			metrics.CommandErrorsTotal.With(prometheus.Labels{"type": "local"}).Inc()
		}
		localLogger.Info("Command completed", "duration", time.Since(start).Seconds())
	}
	if a.Response.SSHCommand != "" {
		if a.Response.SSHHost == "" {
			err := errors.New("must provide SSH host using annotations")
			a.logger.Error("Annotation validation failed", "err", err)
			metrics.ErrorsTotal.Inc()
			return err
		}
		sshLogger := a.logger.With("type", "ssh", "ssh_user", r.SSHUser, "ssh_key", r.SSHKey,
			"ssh_cert", r.SSHCertificate, "ssh_host", r.SSHHost, "command", r.SSHCommand)
		err = a.Response.runSSHCommand(sshLogger)
		if err != nil {
			sshLogger.Error("Failed to run SSH command", "err", err)
			metrics.CommandErrorsTotal.With(prometheus.Labels{"type": "ssh"}).Inc()
		}
		sshLogger.Info("Command completed", "duration", time.Since(start).Seconds())
	}
	return err
}

func (a *Alert) buildResponse(c *config.Config) (AlertResponse, error) {
	r := AlertResponse{
		SSHUser:              c.SSHUser,
		SSHKey:               c.SSHKey,
		SSHPassword:          c.SSHPassword,
		SSHCertificate:       c.SSHCertificate,
		SSHKnownHosts:        c.SSHKnownHosts,
		SSHHostKeyAlgorithms: c.SSHHostKeyAlgorithms,
		SSHConnectionTimeout: c.SSHConnectionTimeout,
		SSHCommandTimeout:    c.SSHCommandTimeout,
		LocalCommandTimeout:  c.LocalCommandTimeout,
	}
	if val, ok := a.Annotations[statusAnnotation]; ok {
		r.Status = strings.Split(val, ",")
	} else {
		r.Status = []string{"firing"}
	}
	if val, ok := a.Annotations[sshUserAnnotation]; ok {
		r.SSHUser = val
	}
	if val, ok := a.Annotations[sshKeyAnnotation]; ok {
		r.SSHKey = val
	}
	if val, ok := a.Annotations[sshCertAnnotation]; ok {
		r.SSHCertificate = val
	}
	if val, ok := a.Annotations[sshHostAnnotation]; ok {
		r.SSHHost = val
	}
	if val, ok := a.Annotations[sshCommandAnnotation]; ok {
		r.SSHCommand = val
	}
	if val, ok := a.Annotations[sshConnTimeout]; ok {
		timeout, err := time.ParseDuration(val)
		if err == nil {
			r.SSHConnectionTimeout = timeout
		} else {
			a.logger.Error("Unable to parse SSH connection timeout", "err", err, "timeout", val)
			return r, err
		}
	}
	if val, ok := a.Annotations[sshCommandTimeout]; ok {
		timeout, err := time.ParseDuration(val)
		if err == nil {
			r.SSHCommandTimeout = timeout
		} else {
			a.logger.Error("Unable to parse SSH command timeout", "err", err, "timeout", val)
			return r, err
		}
	}
	if val, ok := a.Annotations[localCommandAnnotation]; ok {
		r.LocalCommand = val
	}
	if val, ok := a.Annotations[localCommandTimeout]; ok {
		timeout, err := time.ParseDuration(val)
		if err == nil {
			r.LocalCommandTimeout = timeout
		} else {
			a.logger.Error("Unable to parse local command timeout", "err", err, "timeout", val)
			return r, err
		}
	}
	return r, nil
}
