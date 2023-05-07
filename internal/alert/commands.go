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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/treydock/alertmanager-command-responder/internal/metrics"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func (r *AlertResponse) runLocalCommand(logger log.Logger) error {
	var stdout, stderr bytes.Buffer
	localCmd := strings.Split(r.LocalCommand, " ")
	cmdName := localCmd[0]
	var cmdArgs []string
	if len(localCmd) > 1 {
		cmdArgs = localCmd[1:]
	}
	level.Info(logger).Log("msg", "Running local command", "command", cmdName, "args", strings.Join(cmdArgs, " "))
	ctx, cancel := context.WithTimeout(context.Background(), r.LocalCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		level.Error(logger).Log("msg", "Local command timed out")
		return fmt.Errorf("Local command timed out: %s", r.LocalCommand)
	} else if err != nil {
		level.Error(logger).Log("msg", "Error executing command", "err", err)
		return err
	}
	level.Info(logger).Log("msg", "Local command completed", "out", stdout.String(), "err", stderr.String())
	return nil
}

func (r *AlertResponse) runSSHCommand(logger log.Logger) error {
	level.Info(logger).Log("msg", "Running SSH command")
	c1 := make(chan int, 1)
	var auth ssh.AuthMethod
	var err, sessionerror, commanderror error
	var stdout, stderr bytes.Buffer
	timeout := false

	if r.SSHCertificate != "" {
		auth, err = getCertificateAuth(r.SSHKey, r.SSHCertificate)
		if err != nil {
			level.Error(logger).Log("msg", "Error setting up certificate auth", "err", err)
			return err
		}
	} else if r.SSHKey != "" {
		auth, err = getPrivateKeyAuth(r.SSHKey)
		if err != nil {
			level.Error(logger).Log("msg", "Error setting up private key auth", "err", err)
			return err
		}
	} else if r.SSHPassword != "" {
		auth = ssh.Password(r.SSHPassword)
	}
	level.Debug(logger).Log("msg", "Dial SSH", "timeout", r.SSHConnectionTimeout*time.Second)
	sshConfig := &ssh.ClientConfig{
		User:              r.SSHUser,
		Auth:              []ssh.AuthMethod{auth},
		HostKeyCallback:   hostKeyCallback(r.SSHKnownHosts, logger),
		HostKeyAlgorithms: r.SSHHostKeyAlgorithms,
		Timeout:           r.SSHConnectionTimeout,
	}
	connection, err := ssh.Dial("tcp", r.SSHHost, sshConfig)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to establish SSH connection", "err", err)
		return err
	}
	defer connection.Close()

	go func(conn *ssh.Client) {
		var session *ssh.Session
		session, sessionerror = conn.NewSession()
		if sessionerror != nil {
			return
		}
		session.Stdout = &stdout
		session.Stderr = &stderr
		commanderror = session.Run(r.SSHCommand)
		if commanderror != nil {
			return
		}
		if !timeout {
			c1 <- 1
		}
	}(connection)

	select {
	case <-c1:
	case <-time.After(r.SSHCommandTimeout):
		timeout = true
		close(c1)
		level.Error(logger).Log("msg", "Timeout executing SSH command")
		return fmt.Errorf("Timeout executing SSH command: %s", r.SSHCommand)
	}
	close(c1)

	if sessionerror != nil {
		level.Error(logger).Log("msg", "Failed to establish SSH session", "err", sessionerror)
		return sessionerror
	}
	if commanderror != nil {
		level.Error(logger).Log("msg", "Failed to run SSH command", "err", commanderror)
		return commanderror
	}
	level.Info(logger).Log("msg", "SSH command completed", "out", stdout.String(), "err", stderr.String())
	return nil
}

func getPrivateKeyAuth(privatekey string) (ssh.AuthMethod, error) {
	buffer, err := os.ReadFile(privatekey)
	if err != nil {
		return nil, err
	}
	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}

func getCertificateAuth(privatekey string, certificate string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(privatekey)
	if err != nil {
		return nil, fmt.Errorf("Unable to read private key: '%s' %v", privatekey, err)
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse private key: '%s' %v", privatekey, err)
	}

	// Load the certificate
	cert, err := os.ReadFile(certificate)
	if err != nil {
		return nil, fmt.Errorf("Unable to read certificate file: '%s' %v", certificate, err)
	}

	pk, _, _, _, err := ssh.ParseAuthorizedKey(cert)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse public key: '%s' %v", certificate, err)
	}

	certSigner, err := ssh.NewCertSigner(pk.(*ssh.Certificate), signer)
	if err != nil {
		return nil, fmt.Errorf("Unable to create cert signer: %v", err)
	}

	return ssh.PublicKeys(certSigner), nil
}

func hostKeyCallback(knownHosts string, logger log.Logger) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		var hostKeyCallback ssh.HostKeyCallback
		var err error
		if knownHosts != "" {
			publicKey := base64.StdEncoding.EncodeToString(key.Marshal())
			level.Debug(logger).Log("msg", "Verify SSH known hosts", "hostname", hostname, "remote", remote.String(), "key", publicKey)
			hostKeyCallback, err = knownhosts.New(knownHosts)
			if err != nil {
				level.Error(logger).Log("msg", "Error creating hostkeycallback function", "err", err)
				metrics.CommandErrorsTotal.With(prometheus.Labels{"type": "ssh"}).Inc()
				return err
			}
		} else {
			hostKeyCallback = ssh.InsecureIgnoreHostKey()
		}
		return hostKeyCallback(hostname, remote, key)
	}
}
