package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
)

var (
	gitTag    string
	gitSha    string
	buildTime string
	config    Config
	cfgPath   string
)

type (
	Version struct {
		GitTag    string `json:"version"`
		GitSha    string `json:"sha1sum"`
		BuildTime string `json:"buildtime"`
	}

	Config struct {
		SSHKey       string            `yaml:"ssh_key" json:"ssh_key"`
		HostLabel    string            `yaml:"host_label" json:"host_label"`
		CommandLabel string            `yaml:"command_label" json:"command_label"`
		Responders   []ConfigResponder `yaml:"responders" json:"responders"`
		path         string
	}

	ConfigResponder struct {
		Match   []map[string]string `yaml:"match" json:"match"`
		Command string              `yaml:"command" json:"command"`
	}

	HookMessage struct {
		Version           string            `json:"version"`
		GroupKey          string            `json:"groupKey"`
		Status            string            `json:"status"`
		Receiver          string            `json:"receiver"`
		GroupLabels       map[string]string `json:"groupLabels"`
		CommonLabels      map[string]string `json:"commonLabels"`
		CommonAnnotations map[string]string `json:"commonAnnotations"`
		ExternalURL       string            `json:"externalURL"`
		Alerts            []Alert           `json:"alerts"`
	}

	// Alert is a single alert.
	Alert struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt    string            `json:"startsAt,omitempty"`
		EndsAt      string            `json:"EndsAt,omitempty"`
	}

	alertStore struct {
		sync.Mutex
		capacity int
		alerts   []*HookMessage
	}
)

func versionJSON() []byte {
	versionValue := Version{gitTag, gitSha, buildTime}
	jsonValue, _ := json.Marshal(versionValue)
	return jsonValue
}

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

func (c *Config) readConfig() {
	log.Printf("reading config: %s", c.path)
	data, err := ioutil.ReadFile(c.path)
	if err != nil {
		log.Fatal(err)
	}
	if err := c.Parse(data); err != nil {
		log.Fatal(err)
	}
	cfgJson, _ := json.Marshal(c)
	log.Printf("CONFIG: %s\n", cfgJson)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok\n")
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(versionJSON())
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	jsonValue, _ := json.Marshal(config)
	w.Write(jsonValue)
}

func (s *alertStore) alertsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.getHandler(w, r)
	case http.MethodPost:
		s.postHandler(w, r)
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"message": "not found"}`))
}

func (s *alertStore) getHandler(w http.ResponseWriter, r *http.Request) {
	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json")

	s.Lock()
	defer s.Unlock()

	if err := enc.Encode(s.alerts); err != nil {
		log.Printf("error encoding messages: %v", err)
	}
}

func (s *alertStore) postHandler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var m HookMessage
	if err := dec.Decode(&m); err != nil {
		log.Printf("error decoding message: %v", err)
		http.Error(w, "invalid request body", 400)
		return
	}

	s.Lock()

	s.alerts = append(s.alerts, &m)

	if len(s.alerts) > s.capacity {
		a := s.alerts
		_, a = a[0], a[1:]
		s.alerts = a
	}
	s.Unlock()
	alertJson, _ := json.Marshal(m)
	log.Printf("alert = %s", alertJson)
}

func main() {
	flag.StringVar(&cfgPath, "cfg", "", "path to configuration file")
	listenAddr := flag.String("listen", ":10000", "HTTP port to listen on")
	printVersion := flag.Bool("version", false, "if true, print version and exit")

	flag.Parse()

	if *printVersion {
		fmt.Printf("%s\n", versionJSON())
		os.Exit(0)
	}

	if len(cfgPath) == 0 {
		log.Fatal("Must pass -cfg")
	}
	config = Config{
		path: cfgPath,
	}

	config.readConfig()

	s := &alertStore{
		capacity: 32,
	}

	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	exit_chan := make(chan int)
	go func() {
		for {
			s := <-signal_chan
			switch s {
			case syscall.SIGHUP:
				config.readConfig()
			case syscall.SIGINT:
				exit_chan <- 0
			case syscall.SIGTERM:
				exit_chan <- 0
			case syscall.SIGQUIT:
				exit_chan <- 0
			default:
				log.Printf("Unknown signal %d", s)
				exit_chan <- 1
			}
		}
	}()

	r := mux.NewRouter()
	r.HandleFunc("/healthz", healthzHandler).Methods(http.MethodGet)
	r.HandleFunc("/version", versionHandler).Methods(http.MethodGet)
	r.HandleFunc("/config", configHandler).Methods(http.MethodGet)
	r.HandleFunc("/alerts", s.alertsHandler).Methods(http.MethodGet)
	r.HandleFunc("/alerts", s.alertsHandler).Methods(http.MethodPost)
	r.HandleFunc("/", notFound)
	srv := &http.Server{
		Handler:      r,
		Addr:         *listenAddr,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	code := <-exit_chan
	log.Printf("Shutting down")
	os.Exit(code)
}
