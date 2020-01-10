package config

import (
	"encoding/json"
	"io/ioutil"
	"log"

	"github.com/treydock/alertmanager-command-responder/internal/responder"
	"gopkg.in/yaml.v2"
)

type (
	Config struct {
		SSHKey     string                `yaml:"ssh_key" json:"ssh_key"`
		Responders []responder.Responder `yaml:"responders" json:"responders"`
	}
)

func (c *Config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	//if c.Hostname == "" {
	//	return errors.New("Kitchen config: invalid `hostname`")
	//}
	// ... same check for Username, SSHKey, and Port ...
	return nil
}

func (c *Config) ReadConfig(cfgPath string) {
	log.Printf("reading config: %s", cfgPath)
	data, err := ioutil.ReadFile(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := c.Parse(data); err != nil {
		log.Fatal(err)
	}
	cfgJson, _ := json.Marshal(c)
	log.Printf("CONFIG: %s\n", cfgJson)
}
