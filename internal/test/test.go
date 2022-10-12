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

package test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	KnownHosts  *os.File
	TestLock    sync.Mutex
	TestResults = map[string]bool{
		"test1":   false,
		"test1.1": false,
		"test1.2": false,
		"test2":   false,
		"test3":   false,
	}
)

func SSHServer(listen int) (*ssh.Server, error) {
	s := &ssh.Server{
		Addr:             fmt.Sprintf(":%d", listen),
		Handler:          handler,
		PublicKeyHandler: publicKeyHandler,
		PasswordHandler:  passwordHandler,
	}
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("ERROR generating RSA host key: %s", err)
		return s, err
	}
	signer, err := gossh.NewSignerFromKey(hostKey)
	if err != nil {
		fmt.Printf("ERROR generating host key signer: %s", err)
		return s, err
	}
	s.AddHostKey(signer)
	KnownHosts, err = os.CreateTemp("", "knowm_hosts")
	if err != nil {
		fmt.Printf("ERROR creating known hosts: %s", err)
		return s, err
	}
	knownHostsLine := knownhosts.Line([]string{fmt.Sprintf("localhost:%d", listen)}, s.HostSigners[0].PublicKey())
	if _, err = KnownHosts.Write([]byte(knownHostsLine)); err != nil {
		fmt.Printf("ERROR writing known hosts: %s", err)
		return s, err
	}
	return s, nil
}

func handler(s ssh.Session) {
	TestLock.Lock()
	cmd := s.Command()[0]
	if _, ok := TestResults[cmd]; ok {
		TestResults[cmd] = true
	}
	TestLock.Unlock()
	return
}

func FixtureDir() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	return filepath.Join(dir, "fixtures")
}

func publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	pubKeyBuffer, err := os.ReadFile(filepath.Join(FixtureDir(), "id_rsa_test1.pub"))
	if err != nil {
		fmt.Printf("ERROR reading public key id_rsa_test1.pub: %s", err)
		os.Exit(1)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubKeyBuffer)
	if err != nil {
		fmt.Printf("ERROR parsing public key id_rsa_test1.pub: %s", err)
		os.Exit(1)
	}
	pubCertBuffer, err := os.ReadFile(filepath.Join(FixtureDir(), "id_rsa_test1-cert.pub"))
	if err != nil {
		fmt.Printf("ERROR reading public key id_rsa_test1-cert.pub: %s", err)
		os.Exit(1)
	}
	pubCert, _, _, _, err := ssh.ParseAuthorizedKey(pubCertBuffer)
	if err != nil {
		fmt.Printf("ERROR parsing public cert key id_rsa_test1-cert.pub: %s", err)
		os.Exit(1)
	}

	if ssh.KeysEqual(key, pubKey) {
		return true
	} else if ssh.KeysEqual(key, pubCert) {
		return true
	} else {
		return false
	}
}

func passwordHandler(ctx ssh.Context, password string) bool {
	if password == "test" {
		return true
	} else {
		return false
	}
}
