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
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/template"
	"github.com/treydock/alertmanager-command-responder/internal/config"
	"github.com/treydock/alertmanager-command-responder/internal/utils"
)

func TestName(t *testing.T) {
	alert := &Alert{
		Alert: template.Alert{
			Labels:      map[string]string{"alertname": "foo"},
			Fingerprint: "bar",
		},
	}
	if alert.Name() != "foo" {
		t.Errorf("Unexpected value for name, got: %s", alert.Name())
	}
	alert.Alert.Labels = nil
	if alert.Name() != "bar" {
		t.Errorf("Unexpected value for name, got: %s", alert.Name())
	}
}

func TestBuildResponse(t *testing.T) {
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser: "test",
			SSHKey:  "ssh_key",
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	alert := &Alert{
		Alert: template.Alert{
			Labels:      map[string]string{"alertname": "foo"},
			Fingerprint: "bar",
		},
		logger: logger,
	}
	r, err := alert.buildResponse(sc.C)
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
	if len(r.Status) != 1 && r.Status[0] != "firing" {
		t.Errorf("Unexpected value for Status, got %+v", r.Status)
	}
	if r.SSHUser != "test" {
		t.Errorf("Unexpected value for SSHUser, got %s", r.SSHUser)
	}
	if r.SSHKey != "ssh_key" {
		t.Errorf("Unexpected value for SSHKey, got %s", r.SSHKey)
	}
	alert.Alert.Annotations = map[string]string{
		"cr_status":            "firing,resolved",
		"cr_ssh_user":          "foo",
		"cr_ssh_key":           "key",
		"cr_ssh_cert":          "cert",
		"cr_ssh_host":          "host.example.com",
		"cr_ssh_conn_timeout":  "5s",
		"cr_ssh_cmd":           "exit 0",
		"cr_ssh_cmd_timeout":   "10s",
		"cr_local_cmd":         "hostname",
		"cr_local_cmd_timeout": "15s",
	}
	r, err = alert.buildResponse(sc.C)
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
	if len(r.Status) != 2 && !utils.SliceContains(r.Status, "firing") && !utils.SliceContains(r.Status, "resolved") {
		t.Errorf("Unexpected value for Status, got %+v", r.Status)
	}
	if r.SSHUser != "foo" {
		t.Errorf("Unexpected value for SSHUser, got %s", r.SSHUser)
	}
	if r.SSHKey != "key" {
		t.Errorf("Unexpected value for SSHKey, got %s", r.SSHKey)
	}
	if r.SSHCertificate != "cert" {
		t.Errorf("Unexpected value for SSHCertificate, got %s", r.SSHCertificate)
	}
	if r.SSHHost != "host.example.com" {
		t.Errorf("Unexpected value for SSHHost, got %s", r.SSHHost)
	}
	if r.SSHCommand != "exit 0" {
		t.Errorf("Unexpected value for SSHCommand, got %s", r.SSHCommand)
	}
	if r.SSHConnectionTimeout.Seconds() != 5 {
		t.Errorf("Unexpected value for SSHConnectionTimeout, got %f", r.SSHConnectionTimeout.Seconds())
	}
	if r.SSHCommandTimeout.Seconds() != 10 {
		t.Errorf("Unexpected value for SSHCommandTimeout, got %f", r.SSHCommandTimeout.Seconds())
	}
	if r.LocalCommand != "hostname" {
		t.Errorf("Unexpected value for LocalCommand, got %s", r.LocalCommand)
	}
	if r.LocalCommandTimeout.Seconds() != 15 {
		t.Errorf("Unexpected value for LocalCommandTimeout, got %f", r.LocalCommandTimeout.Seconds())
	}
}

func TestBuildResponseErrors(t *testing.T) {
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser: "test",
			SSHKey:  "ssh_key",
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	alert := &Alert{
		Alert: template.Alert{
			Labels: map[string]string{"alertname": "foo"},
			Annotations: map[string]string{
				"cr_ssh_conn_timeout": "foo",
			},
			Fingerprint: "bar",
		},
		logger: logger,
	}
	_, err := alert.buildResponse(sc.C)
	if err == nil {
		t.Errorf("Expected an error")
	}
	alert.Alert.Annotations = map[string]string{
		"cr_ssh_cmd_timeout": "foo",
	}
	_, err = alert.buildResponse(sc.C)
	if err == nil {
		t.Errorf("Expected an error")
	}
	alert.Alert.Annotations = map[string]string{
		"cr_local_cmd_timeout": "foo",
	}
	_, err = alert.buildResponse(sc.C)
	if err == nil {
		t.Errorf("Expected an error")
	}
}
