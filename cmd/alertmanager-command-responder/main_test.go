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
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/template"
	"github.com/treydock/alertmanager-command-responder/internal/config"
	"github.com/treydock/alertmanager-command-responder/internal/test"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var sshPort = 2221

func TestMain(m *testing.M) {
	s, err := test.SSHServer(sshPort)
	if err != nil {
		fmt.Printf("ERROR getting SSH server: %s", err)
		os.Exit(1)
	}
	defer os.Remove(test.KnownHosts.Name())

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
			SSHKey:  filepath.Join(test.FixtureDir(), "id_rsa_test1"),
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
					"cr_ssh_cmd":         "test1",
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

	test.TestLock.Lock()
	if !test.TestResults["test1"] {
		t.Errorf("Test1 was not executed")
	}
	test.TestResults["test1"] = false
	test.TestLock.Unlock()

	// Test setting ssh_key via annotation
	sc.C.SSHKey = ""
	data = template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_key":         filepath.Join(test.FixtureDir(), "id_rsa_test1"),
					"cr_ssh_cmd":         "test1",
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

	test.TestLock.Lock()
	if !test.TestResults["test1"] {
		t.Errorf("Test1 was not executed")
	}
	test.TestResults["test1"] = false
	test.TestLock.Unlock()
}

func TestRunStatus(t *testing.T) {
	port := "10002"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser: "test",
			SSHKey:  filepath.Join(test.FixtureDir(), "id_rsa_test1"),
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

	test.TestLock.Lock()
	if !test.TestResults["test1.1"] {
		t.Errorf("Test1.1 was not executed")
	}
	if !test.TestResults["test1.2"] {
		t.Errorf("Test1.2 was not executed")
	}
	test.TestResults["test1.1"] = false
	test.TestResults["test1.2"] = false
	test.TestLock.Unlock()

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

	test.TestLock.Lock()
	if !test.TestResults["test1.1"] {
		t.Errorf("Test1.1 was not executed")
	}
	if test.TestResults["test1.2"] {
		t.Errorf("Test1.2 was executed")
	}
	test.TestResults["test1.1"] = false
	test.TestResults["test1.2"] = false
	test.TestLock.Unlock()
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

	test.TestLock.Lock()
	if !test.TestResults["test2"] {
		t.Errorf("Test2 was not executed")
	}
	test.TestResults["test2"] = false
	test.TestLock.Unlock()

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

	test.TestLock.Lock()
	if test.TestResults["test2"] {
		t.Errorf("Test2 was executed")
	}
	test.TestResults["test2"] = false
	test.TestLock.Unlock()
}

func TestRunCert(t *testing.T) {
	port := "10004"
	if _, err := kingpin.CommandLine.Parse([]string{fmt.Sprintf("--web.listen-address=:%s", port)}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			SSHUser:        "test",
			SSHKey:         filepath.Join(test.FixtureDir(), "id_rsa_test1"),
			SSHCertificate: filepath.Join(test.FixtureDir(), "id_rsa_test1-cert.pub"),
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

	test.TestLock.Lock()
	if !test.TestResults["test3"] {
		t.Errorf("Test3 was not executed")
	}
	test.TestResults["test3"] = false
	test.TestLock.Unlock()

	// Test setting ssh_certificate via annotation
	sc.C.SSHKey = ""
	sc.C.SSHCertificate = ""
	data = template.Data{
		Alerts: []template.Alert{
			template.Alert{
				Status: "firing",
				Annotations: template.KV{
					"cr_ssh_host":        fmt.Sprintf("localhost:%d", sshPort),
					"cr_ssh_key":         filepath.Join(test.FixtureDir(), "id_rsa_test1"),
					"cr_ssh_cert":        filepath.Join(test.FixtureDir(), "id_rsa_test1-cert.pub"),
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

	test.TestLock.Lock()
	if !test.TestResults["test3"] {
		t.Errorf("Test3 was not executed")
	}
	test.TestResults["test3"] = false
	test.TestLock.Unlock()
}
