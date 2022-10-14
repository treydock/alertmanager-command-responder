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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/treydock/alertmanager-command-responder/internal/config"
	"github.com/treydock/alertmanager-command-responder/internal/metrics"
	"github.com/treydock/alertmanager-command-responder/internal/utils"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var sshPort = 2221

func TestMain(m *testing.M) {
	s, err := SSHServer(sshPort)
	if err != nil {
		fmt.Printf("ERROR getting SSH server: %s", err)
		os.Exit(1)
	}
	defer os.Remove(KnownHosts.Name())

	go func() {
		if err := s.ListenAndServe(); err != nil {
			fmt.Printf("ERROR starting SSH server: %s", err)
			os.Exit(1)
		}
	}()

	exitVal := m.Run()

	os.Exit(exitVal)
}

func TestRun(t *testing.T) {
	port := "10001"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser: "test",
			SSHKey:  filepath.Join(FixtureDir(), "id_rsa_test1"),
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	tmp, err := os.CreateTemp("", "file")
	if err != nil {
		t.Errorf("Unable to create temp file: %s", err)
	}
	if _, err := tmp.Write([]byte("test")); err != nil {
		t.Errorf("Unable to write to temp file: %s", err)
	}
	if err := tmp.Close(); err != nil {
		t.Errorf("Unable to close temp file: %s", err)
	}
	go run(sc, logger)

	data := template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test0.0",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_local_cmd":         fmt.Sprintf("rm -f %s", tmp.Name()),
					"cr_local_cmd_timeout": "2s",
				},
				Fingerprint: "test-command",
			},
		},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test0.0"] {
		t.Errorf("Test1 was not executed")
	}
	TestResults["test0.0"] = false
	TestLock.Unlock()
	if utils.FileExists(tmp.Name()) {
		t.Errorf("Test command file was not removed")
	}

	// Test setting ssh_key and ssh_user via annotation
	sc.C.SSHUser = ""
	sc.C.SSHKey = ""
	data = template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_user":        "test",
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_key":         filepath.Join(FixtureDir(), "id_rsa_test1"),
					"cr_ssh_cmd":         "test0.0",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_user":        "test",
					"cr_ssh_key":         filepath.Join(FixtureDir(), "id_rsa_test1"),
					"cr_ssh_cmd":         "test0.1",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test-missing-host",
			},
		},
	}

	jsonData, err = json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test0.0"] {
		t.Errorf("Test0.0 was not executed")
	}
	if TestResults["test0.1"] {
		t.Errorf("Test0.1 should not have run")
	}
	TestResults["test0.0"] = false
	TestResults["test0.1"] = false
	TestLock.Unlock()
}

func TestRunStatus(t *testing.T) {
	port := "10002"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser: "test",
			SSHKey:  filepath.Join(FixtureDir(), "id_rsa_test1"),
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	go run(sc, logger)

	data := template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test1.1",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "resolved",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test1.2",
					"cr_ssh_cmd_timeout": "2s",
					"cr_status":          "firing,resolved",
				},
				Fingerprint: "test",
			},
		},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test1.1"] {
		t.Errorf("Test1.1 was not executed")
	}
	if !TestResults["test1.2"] {
		t.Errorf("Test1.2 was not executed")
	}
	TestResults["test1.1"] = false
	TestResults["test1.2"] = false
	TestLock.Unlock()

	// Test resolved alert is skipped
	data = template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test1.1",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "resolved",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test1.2",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
		},
	}
	jsonData, err = json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test1.1"] {
		t.Errorf("Test1.1 was not executed")
	}
	if TestResults["test1.2"] {
		t.Errorf("Test1.2 was executed")
	}
	TestResults["test1.1"] = false
	TestResults["test1.2"] = false
	TestLock.Unlock()
}

func TestRunPassword(t *testing.T) {
	port := "10003"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser:     "test",
			SSHPassword: "test",
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	go run(sc, logger)

	data := template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test2",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
		},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test2"] {
		t.Errorf("Test2 was not executed")
	}
	TestResults["test2"] = false
	TestLock.Unlock()

	// Test incorrect password
	sc.C.SSHPassword = "wrong"
	jsonData, err = json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if TestResults["test2"] {
		t.Errorf("Test2 was executed")
	}
	TestResults["test2"] = false
	TestLock.Unlock()
}

func TestRunCert(t *testing.T) {
	port := "10004"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser:        "test",
			SSHKey:         filepath.Join(FixtureDir(), "id_rsa_test1"),
			SSHCertificate: filepath.Join(FixtureDir(), "id_rsa_test1-cert.pub"),
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	go run(sc, logger)

	data := template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test3",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
		},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test3"] {
		t.Errorf("Test3 was not executed")
	}
	TestResults["test3"] = false
	TestLock.Unlock()

	// Test setting ssh_certificate via annotation
	sc.C.SSHKey = ""
	sc.C.SSHCertificate = ""
	data = template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_key":         filepath.Join(FixtureDir(), "id_rsa_test1"),
					"cr_ssh_cert":        filepath.Join(FixtureDir(), "id_rsa_test1-cert.pub"),
					"cr_ssh_cmd":         "test3",
					"cr_ssh_cmd_timeout": "2s",
				},
				Fingerprint: "test",
			},
		},
	}

	jsonData, err = json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	TestLock.Lock()
	if !TestResults["test3"] {
		t.Errorf("Test3 was not executed")
	}
	TestResults["test3"] = false
	TestLock.Unlock()
}

func TestRunGET(t *testing.T) {
	port := "10005"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	go run(sc, logger)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/healthz", port))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d got %d", http.StatusOK, resp.StatusCode)
	}
	resp, err = http.Get(fmt.Sprintf("http://localhost:%s/version", port))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d got %d", http.StatusOK, resp.StatusCode)
	}
	resp, err = http.Get(fmt.Sprintf("http://localhost:%s/config", port))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d got %d", http.StatusOK, resp.StatusCode)
	}
}

func TestRunMetrics(t *testing.T) {
	port := "10006"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser: "test",
		},
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	go run(sc, logger)
	data := template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test6.1",
					"cr_ssh_cmd_timeout": "2s",
					"cr_ssh_key":         filepath.Join(FixtureDir(), "id_rsa_test1"),
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test6.2",
					"cr_ssh_cmd_timeout": "2s",
					"cr_ssh_key":         "dne",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test6.3",
					"cr_ssh_cmd_timeout": "2s",
					"cr_ssh_cert":        "dne",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":         fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":          "test6.4",
					"cr_ssh_cmd_timeout":  "2s",
					"cr_ssh_key":          filepath.Join(FixtureDir(), "id_rsa_test1"),
					"cr_ssh_conn_timeout": "-2s",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":         fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":          "test6.5",
					"cr_ssh_cmd_timeout":  "2s",
					"cr_ssh_key":          filepath.Join(FixtureDir(), "id_rsa_test1"),
					"cr_ssh_conn_timeout": "foo",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "resolved",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_cmd":         "test6.6",
					"cr_ssh_cmd_timeout": "2s",
					"cr_ssh_key":         filepath.Join(FixtureDir(), "id_rsa_test1"),
					"cr_status":          "firing,resolved",
				},
				Fingerprint: "test",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_local_cmd":         "exit 1",
					"cr_local_cmd_timeout": "2s",
				},
				Fingerprint: "test-command-error",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_local_cmd":         "sleep 1",
					"cr_local_cmd_timeout": "-2s",
				},
				Fingerprint: "test-command-timeout",
			},
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_local_cmd":         "hostname",
					"cr_local_cmd_timeout": "2s",
				},
				Fingerprint: "test-command-single",
			},
		},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	resetCounters()
	_, err = http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)
	expected := `
	# HELP alertmanager_command_responder_command_errors_total Total number of command errors
	# TYPE alertmanager_command_responder_command_errors_total counter
	alertmanager_command_responder_command_errors_total{type="local"} 2
	alertmanager_command_responder_command_errors_total{type="ssh"} 3
	`
	if err := testutil.GatherAndCompare(metrics.Metrics(), strings.NewReader(expected),
		"alertmanager_command_responder_command_errors_total"); err != nil {
		t.Errorf("unexpected collecting result:\n%s", err)
	}
}

func TestRunInvalidJSON(t *testing.T) {
	port := "10007"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	go run(sc, logger)
	resp, err := http.Post(fmt.Sprintf("http://localhost:%s/alerts", port), "application/json", bytes.NewBuffer([]byte("foo")))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d got %d", http.StatusBadRequest, resp.StatusCode)
	}
}

func resetCounters() {
	metrics.CommandErrorsTotal.Reset()
	metrics.CommandErrorsTotal.WithLabelValues("ssh")
	metrics.CommandErrorsTotal.WithLabelValues("local")
}
