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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
)

var (
	namespace = "alertmanager_command_responder"
	BuildInfo = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Build information",
		ConstLabels: prometheus.Labels{
			"version":   version.Version,
			"revision":  version.Revision,
			"branch":    version.Branch,
			"builddate": version.BuildDate,
			"goversion": version.GoVersion,
		},
	})
	CommandErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "command_errors_total",
		Help:      "Total number of SSH command errors",
	}, []string{"type"})
)

func MetricsInit() {
	BuildInfo.Set(1)
	CommandErrorsTotal.WithLabelValues("ssh")
	CommandErrorsTotal.WithLabelValues("local")
}

func Metrics() prometheus.Gatherers {
	registry := prometheus.NewRegistry()
	registry.MustRegister(BuildInfo)
	registry.MustRegister(CommandErrorsTotal)
	gatherers := prometheus.Gatherers{registry}
	gatherers = append(gatherers, prometheus.DefaultGatherer)
	return gatherers
}
