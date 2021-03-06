package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

var (
	config     Config
	cfgPath    = kingpin.Flag("cfg", "path to configuration file").Default("").String()
	listenAddr = kingpin.Flag("listen", "HTTP port to listen on").Default(":10000").String()
	interval   = kingpin.Flag("interval", "Interval in seconds to operate on received alerts").Default("60").Int()
	state      = kingpin.Flag("state", "Path to saved state file").Default("").String()
)

type (
	Version struct {
		Version   string `json:"version"`
		Revision  string `json:"revision"`
		BuildDate string `json:"builddate"`
	}

	Config struct {
		User         string            `yaml:"user" json:"user"`
		SSHKey       string            `yaml:"ssh_key" json:"ssh_key"`
		HostLabel    string            `yaml:"host_label" json:"host_label"`
		CommandLabel string            `yaml:"command_label" json:"command_label"`
		Responders   []ConfigResponder `yaml:"responders" json:"responders"`
		path         string
	}

	ConfigResponder struct {
		Match   []map[string]string `yaml:"match" json:"match"`
		Command string              `yaml:"command" json:"command"`
		User    string              `yaml:"user" json:"user"`
		SSHKey  string              `yaml:"ssh_key" json:"ssh_key"`
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
		capacity int            `json:"alerts"`
		state    string         `json:"state"`
		Alerts   []*HookMessage `json:"alerts"`
	}
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func versionJSON() []byte {
	versionValue := Version{version.Version, version.Revision, version.BuildDate}
	jsonValue, _ := json.Marshal(versionValue)
	return jsonValue
}

func (c *Config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if c.User == "" {
		u, err := user.Current()
		if err != nil {
			log.Errorln("error getting user:", err)
		}
		c.User = u.Username
	}
	if c.SSHKey != "" {
		if !fileExists(c.SSHKey) {
			log.Fatalf("SSH key %s does not exist!", c.SSHKey)
		}
	}
	for idx, r := range c.Responders {
		if r.User == "" {
			c.Responders[idx].User = c.User
		}
		if r.SSHKey == "" {
			c.Responders[idx].SSHKey = c.SSHKey
		}
		if c.Responders[idx].SSHKey != "" {
			if !fileExists(c.Responders[idx].SSHKey) {
				log.Fatalf("SSH key %s does not exist!", c.Responders[idx].SSHKey)
			}
		}
	}
	//	return errors.New("Kitchen config: invalid `hostname`")
	//}
	// ... same check for Username, SSHKey, and Port ...
	return nil
}

func (c *Config) readConfig() {
	log.Infoln("reading config:", c.path)
	data, err := ioutil.ReadFile(c.path)
	if err != nil {
		log.Fatal(err)
	}
	if err := c.Parse(data); err != nil {
		log.Fatal(err)
	}
	cfgJson, _ := json.Marshal(c)
	log.Debugf("CONFIG: %s\n", cfgJson)
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
	case http.MethodDelete:
		s.deleteHandler(w, r)
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

	if err := enc.Encode(s.Alerts); err != nil {
		log.Errorln("error encoding messages:", err)
	}
}

func (s *alertStore) postHandler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var m HookMessage
	if err := dec.Decode(&m); err != nil {
		log.Errorln("error decoding message:", err)
		http.Error(w, "invalid request body", 400)
		return
	}

	s.Lock()
	defer s.Unlock()

	s.Alerts = append(s.Alerts, &m)

	if len(s.Alerts) > s.capacity {
		a := s.Alerts
		_, a = a[0], a[1:]
		s.Alerts = a
	}
	log.Infof("Received %d alerts\n", len(m.Alerts))
	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, "created\n")
}

func (s *alertStore) deleteHandler(w http.ResponseWriter, r *http.Request) {
	s.Lock()
	s.Alerts = nil
	s.Unlock()
	w.WriteHeader(http.StatusAccepted)
	io.WriteString(w, "deleted\n")
}

func (s *alertStore) saveState() error {
	if s.state == "" {
		return nil
	}
	log.Infoln("Saving state to", s.state)
	s.Lock()
	defer s.Unlock()
	jsonData, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		log.Errorln("generating state:", err)
		return err
	}
	jsonFile, err := os.Create(s.state)
	defer jsonFile.Close()
	if err != nil {
		log.Errorln("saving state:", s.state, err)
		return err
	}
	jsonFile.Write(jsonData)
	return nil
}

func (s *alertStore) loadState() error {
	if s.state == "" {
		return nil
	}
	if !fileExists(s.state) {
		log.Infof("State file %s does not exist", s.state)
		return nil
	}
	s.Lock()
	defer s.Unlock()
	log.Infoln("Loading state from", s.state)
	jsonFile, err := os.Open(s.state)
	if err != nil {
		log.Errorln("loading state:", s.state, err)
		return err
	}
	defer jsonFile.Close()
	err = json.NewDecoder(jsonFile).Decode(&s)
	if err != nil {
		log.Errorln("Parsing JSON state:", err)
		return err
	}
	return nil
}

func (s *alertStore) processAlerts() error {
	log.Infof("Processing %d alerts\n", len(s.Alerts))
	s.Lock()
	defer s.Unlock()

	return nil
}

func main() {
	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print("alertmanager-command-responder"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	/*if *printVersion {
		//log.Println(versionJSON())
		version.Print("alertmanager-command-responder")
		os.Exit(0)
	}*/

	if len(*cfgPath) == 0 {
		log.Fatal("Must pass -cfg")
	}
	config = Config{
		path: *cfgPath,
	}
	config.readConfig()

	s := &alertStore{
		capacity: 32,
		state:    *state,
	}
	s.loadState()

	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	exit_chan := make(chan int)
	stop := make(chan bool)
	go func() {
		for {
			sig := <-signal_chan
			switch sig {
			case syscall.SIGHUP:
				config.readConfig()
				s.saveState()
			case syscall.SIGINT:
				stop <- true
				exit_chan <- 0
			case syscall.SIGTERM:
				stop <- true
				exit_chan <- 0
			case syscall.SIGQUIT:
				stop <- true
				exit_chan <- 0
			default:
				log.Errorln("Unknown signal", sig)
				stop <- true
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
	r.HandleFunc("/alerts", s.alertsHandler).Methods(http.MethodDelete)
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

	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	go func() {
		for {
			select {
			case <-stop:
				log.Infoln("Shutting down ticker")
				return
			case t := <-ticker.C:
				log.Infoln("Processing alert store at", t)
				s.processAlerts()
			}
		}
	}()

	code := <-exit_chan
	log.Info("Shutting down")
	s.saveState()
	ticker.Stop()
	os.Exit(code)
}
