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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/treydock/alertmanager-command-responder/internal/alert"
	"github.com/treydock/alertmanager-command-responder/internal/config"
	"github.com/treydock/alertmanager-command-responder/internal/metrics"
)

var (
	configPath = kingpin.Flag("config.file", "path to configuration file").Default("alertmanager-command-responder.yaml").String()
	listenAddr = kingpin.Flag("web.listen-address", "HTTP port to listen on").Default(":10000").String()
)

func init() {
	metrics.MetricsInit()
}

type Version struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	BuildDate string `json:"builddate"`
}

type JSONResponse struct {
	Status     string      `json:"status"`
	StatusCode int         `json:"statusCode"`
	Message    string      `json:"message,omitempty"`
	Data       interface{} `json:"data,omitempty"`
	logger     log.Logger
}

func asJSON(w http.ResponseWriter, response JSONResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		level.Error(response.logger).Log("msg", "error encoding response", "err", err)
		json.NewEncoder(w).Encode(JSONResponse{Status: "error", StatusCode: http.StatusBadRequest, Message: err.Error()})
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	asJSON(w, JSONResponse{Status: "ok", StatusCode: http.StatusOK})
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	v := Version{version.Version, version.Revision, version.BuildDate}
	asJSON(w, JSONResponse{Status: "ok", StatusCode: http.StatusOK, Data: v})
}

func configHandler(w http.ResponseWriter, r *http.Request, c *config.Config) {
	asJSON(w, JSONResponse{Status: "ok", StatusCode: http.StatusOK, Data: c})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	asJSON(w, JSONResponse{Status: "error", StatusCode: http.StatusNotFound, Message: "not found"})
}

/*func getAlertHandler(w http.ResponseWriter, r *http.Request, c *config.Config) {
	asJSON(w, JSONResponse{Status: "success", StatusCode: http.StatusOK, Data: s.Alerts, logger: s.Logger})
}*/

func postAlertHandler(w http.ResponseWriter, r *http.Request, c *config.Config, logger log.Logger) {
	defer r.Body.Close()
	var data template.Data
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		level.Error(logger).Log("msg", "error decoding message", "err", err)
		metrics.ErrorsTotal.Inc()
		asJSON(w, JSONResponse{Status: "error", StatusCode: http.StatusBadRequest, Message: err.Error()})
		return
	}
	level.Info(logger).Log("msg", fmt.Sprintf("Received %d alerts", len(data.Alerts)))
	asJSON(w, JSONResponse{Status: "success", StatusCode: http.StatusCreated})

	for _, a := range data.Alerts {
		go func(a template.Alert) {
			newAlert := alert.Alert{
				Alert: a,
			}
			err := newAlert.HandleAlert(c, logger)
			if err != nil {
				level.Error(logger).Log("msg", "Error handling alert", "err", err, "fingerprint", a.Fingerprint)
				metrics.ErrorsTotal.Inc()
			}
		}(a)
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	gatherers := metrics.Metrics()
	h := promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

func run(sc *config.SafeConfig, logger log.Logger) int {
	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	exit_chan := make(chan int)
	go func() {
		for {
			sig := <-signal_chan
			switch sig {
			case syscall.SIGHUP:
				err := sc.ReadConfig()
				if err != nil {
					level.Error(logger).Log("msg", "Failed to load configuration file, using old config.")
					metrics.ErrorsTotal.Inc()
				}
			case syscall.SIGINT:
				exit_chan <- 0
			case syscall.SIGTERM:
				exit_chan <- 0
			case syscall.SIGQUIT:
				exit_chan <- 0
			default:
				level.Error(logger).Log("msg", "Unknown signal", "signal", sig)
				exit_chan <- 1
			}
		}
	}()

	r := mux.NewRouter()
	r.HandleFunc("/healthz", healthzHandler).Methods(http.MethodGet)
	r.HandleFunc("/version", versionHandler).Methods(http.MethodGet)
	r.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		configHandler(w, r, sc.C)
	}).Methods(http.MethodGet)
	r.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		postAlertHandler(w, r, sc.C, logger)
	}).Methods(http.MethodPost)
	r.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metricsHandler(w, r)
	}).Methods(http.MethodGet)
	r.HandleFunc("/", notFound)
	srv := &http.Server{
		Handler:      r,
		Addr:         *listenAddr,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			level.Error(logger).Log("msg", "Unable to start HTTP server", "err", err)
			os.Exit(1)
		}
	}()

	code := <-exit_chan
	level.Info(logger).Log("msg", "Shutting down")
	return code
}

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("alertmanager-command-responder"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(promlogConfig)
	sc := config.NewSafeConfig(*configPath, logger)
	err := sc.ReadConfig()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to load configuration file, exiting.")
		os.Exit(1)
	}

	e := run(sc, logger)
	os.Exit(e)
}
