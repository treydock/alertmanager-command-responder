// Copyright 2020 Trey Dockendorf
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

package config

import (
	"os"
	"os/user"
	"testing"
	"time"

	"github.com/go-kit/log"
)

func TestReloadConfigDefaults(t *testing.T) {
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	sc := NewSafeConfig("testdata/config.yaml", logger)
	err := sc.ReadConfig()
	if err != nil {
		t.Errorf("Unexpected err: %s", err.Error())
		return
	}
	if sc.C.SSHUser != "prometheus" {
		t.Errorf("User does not match prometheus")
	}
	duration1, _ := time.ParseDuration("5s")
	duration2, _ := time.ParseDuration("10s")
	if sc.C.SSHConnectionTimeout != duration1 {
		t.Errorf("SSHConnectionTimeout does not match default 5s")
	}
	if sc.C.SSHCommandTimeout != duration2 {
		t.Errorf("SSHCommandTimeout does not match default 10s")
	}
	if sc.C.LocalCommandTimeout != duration2 {
		t.Errorf("LocalCommandTimeout does not match default 10s")
	}
	sc = NewSafeConfig("testdata/config-empty.yaml", logger)
	u, err := user.Current()
	if err != nil {
		t.Errorf("error getting current user: %s", err)
	}
	currentUser := u.Username
	err = sc.ReadConfig()
	if err != nil {
		t.Errorf("Unexpected err: %s", err.Error())
		return
	}
	if sc.C.SSHUser != currentUser {
		t.Errorf("User does not match current user. Execpted: %s\nGot: %s", currentUser, sc.C.SSHUser)
	}
}

func TestReloadConfigBadConfigs(t *testing.T) {
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	tests := []struct {
		ConfigFile    string
		ExpectedError string
	}{
		{
			ConfigFile:    "/dne",
			ExpectedError: "open /dne: no such file or directory",
		},
		{
			ConfigFile:    "testdata/invalid-ssh_key.yaml",
			ExpectedError: "SSH key does not exist: dne",
		},
		{
			ConfigFile:    "testdata/invalid-ssh_cert.yaml",
			ExpectedError: "SSH certificate does not exist: dne",
		},
		{
			ConfigFile:    "testdata/invalid-known_hosts.yaml",
			ExpectedError: "SSH known hosts does not exist: dne",
		},
		{
			ConfigFile:    "testdata/unknown-field.yaml",
			ExpectedError: "yaml: unmarshal errors:\n  line 5: field invalid_extra_field not found in type config.Config",
		},
	}
	for i, test := range tests {
		sc := NewSafeConfig(test.ConfigFile, logger)
		err := sc.ReadConfig()
		if err == nil {
			t.Errorf("In case %v:\nExpected:\n%v\nGot:\nnil", i, test.ExpectedError)
			continue
		}
		if err.Error() != test.ExpectedError {
			t.Errorf("In case %v:\nExpected:\n%v\nGot:\n%v", i, test.ExpectedError, err.Error())
		}
	}
}
