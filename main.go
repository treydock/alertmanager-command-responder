package main

import (
    "flag"
    "fmt"
    "encoding/json"
    "io/ioutil"
    "log"
    "net/http"
    "os"

    "github.com/gorilla/mux"
    "gopkg.in/yaml.v2"
)

var (
  gitTag string
  gitSha string
  buildTime string
  config Config
)

type Version struct {
  GitTag string `json:"version"`
  GitSha string `json:"sha1sum"`
  BuildTime string `json:"buildtime"`
}

type Config struct {
  SSHKey string `yaml:"ssh_key" json:"ssh_key"`
  Responders []Responder `yaml:"responders" json:"responders"`
}

type Responder struct {
  Host string
}

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

func versionRespond(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write(versionJSON())
}

func configRespond(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    jsonValue, _ := json.Marshal(config)
    w.Write(jsonValue)
}

func alertRespond(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    w.Write([]byte(`{"message": "post called"}`))
}

func notFound(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusNotFound)
    w.Write([]byte(`{"message": "not found"}`))
}

func main() {
    cfgPath := flag.String("cfg", "", "path to configuration file")
    listenAddr := flag.String("listen", ":10000", "HTTP port to listen on")
    printVersion := flag.Bool("version", false, "if true, print version and exit")

    flag.Parse()
 
    if *printVersion {
        fmt.Printf("%s\n", versionJSON())
        os.Exit(0)
    }

    if len(*cfgPath) == 0 {
      log.Fatal("Must pass -cfg")
    }

    data, err := ioutil.ReadFile(*cfgPath)
	    if err != nil {
		  log.Fatal(err)
	  }
	  if err := config.Parse(data); err != nil {
		  log.Fatal(err)
	  }
	  fmt.Printf("%+v\n", config)

    r := mux.NewRouter()
    r.HandleFunc("/version", versionRespond).Methods(http.MethodGet)
    r.HandleFunc("/config", configRespond).Methods(http.MethodGet)
    r.HandleFunc("/alert", alertRespond).Methods(http.MethodPost)
    r.HandleFunc("/", notFound)
    log.Fatal(http.ListenAndServe(*listenAddr, r))
}
