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

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/treydock/alertmanager-command-responder/internal/utils"
	yaml "gopkg.in/yaml.v3"
)

const (
	defaultSSHConnectionTimeout = "5s"
	defaultSSHCommandTimeout    = "10s"
	defaultLocalCommandTimeout  = "10s"
)

type SafeConfig struct {
	path   string
	logger log.Logger
	mu     sync.Mutex
	C      *Config
}

type Config struct {
	SSHUser              string        `yaml:"ssh_user" json:"ssh_user"`
	SSHKey               string        `yaml:"ssh_key" json:"ssh_key"`
	SSHPassword          string        `yaml:"ssh_password" json:"ssh_password"`
	SSHCertificate       string        `yaml:"ssh_certificate" json:"ssh_certificate"`
	SSHKnownHosts        string        `yaml:"ssh_known_hosts" json:"ssh_known_hosts"`
	SSHHostKeyAlgorithms []string      `yaml:"ssh_host_key_algorithms" json:"ssh_host_key_algorithms"`
	SSHConnectionTimeout time.Duration `yaml:"ssh_connection_timeout" json:"ssh_connection_timeout"`
	SSHCommandTimeout    time.Duration `yaml:"ssh_command_timeout" json:"ssh_command_timeout"`
	LocalCommandTimeout  time.Duration `yaml:"local_command_timeout" json:"local_command_timeout"`
}

func NewSafeConfig(path string, logger log.Logger) *SafeConfig {
	return &SafeConfig{
		path:   path,
		logger: logger,
	}
}

func (sc *SafeConfig) ParseConfig() error {
	var c = &Config{}
	yamlReader, err := os.Open(sc.path)
	if err != nil {
		level.Error(sc.logger).Log("msg", "Error reading config file", "path", sc.path, "err", err)
		return err
	}
	defer yamlReader.Close()
	decoder := yaml.NewDecoder(yamlReader)
	decoder.KnownFields(true)
	if err := decoder.Decode(c); err != nil {
		level.Error(sc.logger).Log("msg", "Error parsing config file", "path", sc.path, "err", err)
		return err
	}
	if c.SSHUser == "" {
		u, err := user.Current()
		if err != nil {
			level.Error(sc.logger).Log("msg", "error getting current user", "err", err)
			return err
		}
		c.SSHUser = u.Username
	}
	if c.SSHKey != "" {
		if !utils.FileExists(c.SSHKey) {
			level.Error(sc.logger).Log("msg", "SSH key does not exist", "sshkey", c.SSHKey)
			return fmt.Errorf("SSH key does not exist: %s", c.SSHKey)
		}
	}
	if c.SSHCertificate != "" {
		if !utils.FileExists(c.SSHCertificate) {
			level.Error(sc.logger).Log("msg", "SSH certificate does not exist", "ssh_certificate", c.SSHCertificate)
			return fmt.Errorf("SSH certificate does not exist: %s", c.SSHCertificate)
		}
	}
	if c.SSHKnownHosts != "" {
		if !utils.FileExists(c.SSHKnownHosts) {
			level.Error(sc.logger).Log("msg", "SSH known hosts does not exist", "path", c.SSHKnownHosts)
			return fmt.Errorf("SSH known hosts does not exist: %s", c.SSHKnownHosts)
		}
	}
	if c.SSHConnectionTimeout == 0 {
		c.SSHConnectionTimeout, _ = time.ParseDuration(defaultSSHConnectionTimeout)
	}
	if c.SSHCommandTimeout == 0 {
		c.SSHCommandTimeout, _ = time.ParseDuration(defaultSSHCommandTimeout)
	}
	if c.LocalCommandTimeout == 0 {
		c.LocalCommandTimeout, _ = time.ParseDuration(defaultLocalCommandTimeout)
	}

	sc.mu.Lock()
	sc.C = c
	sc.mu.Unlock()
	return nil
}

func (sc *SafeConfig) ReadConfig() error {
	level.Info(sc.logger).Log("msg", "reading config", "path", sc.path)
	if err := sc.ParseConfig(); err != nil {
		return err
	}
	cfgJson, _ := json.Marshal(sc.C)
	level.Debug(sc.logger).Log("msg", "parsed config", "config", cfgJson)
	return nil
}
