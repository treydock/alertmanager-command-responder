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
	if _, err := kingpin.CommandLine.Parse([]string{"--web.listen-address=:10001"}); err != nil {
		t.Fatal(err)
	}
	sc := &config.SafeConfig{
		C: &config.Config{
			User:   "test",
			SSHKey: filepath.Join(test.FixtureDir(), "id_rsa_test1"),
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
					"command_responder_ssh_host":            fmt.Sprintf("localhost:%d", sshPort),
					"command_responder_ssh_command":         "test1",
					"command_responder_ssh_command_timeout": "2s",
				},
				Fingerprint: "test",
			},
		},
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Errorf("Unexpected error generating JSON data: %s", err)
	}
	_, err = http.Post("http://localhost:10001/alerts", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Errorf("Unexpected error making POST request: %s", err)
	}
	time.Sleep(2 * time.Second)

	test.TestLock.Lock()
	if !test.Test1 {
		t.Errorf("Test1 was not executed")
	}
	test.TestLock.Unlock()
}
