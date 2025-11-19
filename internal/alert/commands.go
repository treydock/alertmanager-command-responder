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
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/treydock/alertmanager-command-responder/internal/metrics"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func (r *AlertResponse) runLocalCommand(logger *slog.Logger) error {
	var stdout, stderr bytes.Buffer
	localCmd := strings.Split(r.LocalCommand, " ")
	cmdName := localCmd[0]
	var cmdArgs []string
	if len(localCmd) > 1 {
		cmdArgs = localCmd[1:]
	}
	logger.Info("Running local command", "command", cmdName, "args", strings.Join(cmdArgs, " "))
	ctx, cancel := context.WithTimeout(context.Background(), r.LocalCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		logger.Error("Local command timed out")
		return fmt.Errorf("local command timed out: %s", r.LocalCommand)
	} else if err != nil {
		logger.Error("Error executing command", "err", err)
		return err
	}
	logger.Info("Local command completed", "out", stdout.String(), "err", stderr.String())
	return nil
}

func (r *AlertResponse) runSSHCommand(logger *slog.Logger) error {
	logger.Info("Running SSH command")
	c1 := make(chan int, 1)
	var auth ssh.AuthMethod
	var err, sessionerror, commanderror error
	var stdout, stderr bytes.Buffer

	if r.SSHCertificate != "" {
		auth, err = getCertificateAuth(r.SSHKey, r.SSHCertificate)
		if err != nil {
			logger.Error("Error setting up certificate auth", "err", err)
			return err
		}
	} else if r.SSHKey != "" {
		auth, err = getPrivateKeyAuth(r.SSHKey)
		if err != nil {
			logger.Error("Error setting up private key auth", "err", err)
			return err
		}
	} else if r.SSHPassword != "" {
		auth = ssh.Password(r.SSHPassword)
	}
	logger.Debug("Dial SSH", "timeout", r.SSHConnectionTimeout*time.Second)
	sshConfig := &ssh.ClientConfig{
		User:              r.SSHUser,
		Auth:              []ssh.AuthMethod{auth},
		HostKeyCallback:   hostKeyCallback(r.SSHKnownHosts, logger),
		HostKeyAlgorithms: r.SSHHostKeyAlgorithms,
		Timeout:           r.SSHConnectionTimeout,
	}
	connection, err := ssh.Dial("tcp", r.SSHHost, sshConfig)
	if err != nil {
		logger.Error("Failed to establish SSH connection", "err", err)
		return err
	}
	defer connection.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func(conn *ssh.Client) {
		var session *ssh.Session
		session, sessionerror = conn.NewSession()
		if sessionerror != nil {
			return
		}
		session.Stdout = &stdout
		session.Stderr = &stderr
		commanderror = session.Run(r.SSHCommand)
		select {
		default:
			c1 <- 1
		case <-ctx.Done():
			return
		}
	}(connection)

	select {
	case <-c1:
	case <-time.After(r.SSHCommandTimeout):
		close(c1)
		logger.Error("Timeout executing SSH command")
		return fmt.Errorf("timeout executing SSH command: %s", r.SSHCommand)
	}
	close(c1)

	if sessionerror != nil {
		logger.Error("Failed to establish SSH session", "err", sessionerror)
		return sessionerror
	}
	if commanderror != nil {
		logger.Error("Failed to run SSH command", "err", commanderror, "out", stdout.String(), "err", stderr.String())
		return commanderror
	}
	logger.Info("SSH command completed", "out", stdout.String(), "err", stderr.String())
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
		return nil, fmt.Errorf("unable to read private key: '%s' %v", privatekey, err)
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: '%s' %v", privatekey, err)
	}

	// Load the certificate
	cert, err := os.ReadFile(certificate)
	if err != nil {
		return nil, fmt.Errorf("unable to read certificate file: '%s' %v", certificate, err)
	}

	pk, _, _, _, err := ssh.ParseAuthorizedKey(cert)
	if err != nil {
		return nil, fmt.Errorf("unable to parse public key: '%s' %v", certificate, err)
	}

	certSigner, err := ssh.NewCertSigner(pk.(*ssh.Certificate), signer)
	if err != nil {
		return nil, fmt.Errorf("unable to create cert signer: %v", err)
	}

	return ssh.PublicKeys(certSigner), nil
}

func hostKeyCallback(knownHosts string, logger *slog.Logger) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		var hostKeyCallback ssh.HostKeyCallback
		var err error
		if knownHosts != "" {
			publicKey := base64.StdEncoding.EncodeToString(key.Marshal())
			logger.Debug("Verify SSH known hosts", "hostname", hostname, "remote", remote.String(), "key", publicKey)
			hostKeyCallback, err = knownhosts.New(knownHosts)
			if err != nil {
				logger.Error("Error creating hostkeycallback function", "err", err)
				metrics.CommandErrorsTotal.With(prometheus.Labels{"type": "ssh"}).Inc()
				return err
			}
		} else {
			hostKeyCallback = ssh.InsecureIgnoreHostKey()
		}
		return hostKeyCallback(hostname, remote, key)
	}
}
