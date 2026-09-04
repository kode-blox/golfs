// Copyright 2026 Sayak Mukhopadhyay
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package observability defines GOLFS metrics and HTTP instrumentation.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics contains the private Prometheus registry and low-cardinality collectors.
type Metrics struct {
	Registry *prometheus.Registry

	HTTPRequests   *prometheus.CounterVec
	HTTPDuration   *prometheus.HistogramVec
	AuthCache      *prometheus.CounterVec
	GitHubRequests *prometheus.CounterVec
	GitHubDuration *prometheus.HistogramVec
	S3Requests     *prometheus.CounterVec
	S3Duration     *prometheus.HistogramVec
	BatchObjects   *prometheus.CounterVec
	VerifyResults  *prometheus.CounterVec
}

// New constructs and registers all GOLFS collectors.
func New(version, commit string) *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		Registry:       registry,
		HTTPRequests:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "golfs_http_requests_total", Help: "HTTP requests handled by GOLFS."}, []string{"listener", "route", "method", "status"}),
		HTTPDuration:   prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "golfs_http_request_duration_seconds", Help: "HTTP request duration."}, []string{"listener", "route", "method"}),
		AuthCache:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "golfs_auth_cache_requests_total", Help: "Authorization cache outcomes."}, []string{"result"}),
		GitHubRequests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "golfs_github_requests_total", Help: "GitHub API request outcomes."}, []string{"operation", "status"}),
		GitHubDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "golfs_github_request_duration_seconds", Help: "GitHub API request duration."}, []string{"operation"}),
		S3Requests:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "golfs_s3_requests_total", Help: "S3 API request outcomes."}, []string{"operation", "status"}),
		S3Duration:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "golfs_s3_request_duration_seconds", Help: "S3 API request duration."}, []string{"operation"}),
		BatchObjects:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "golfs_batch_objects_total", Help: "Objects returned by Batch operations."}, []string{"operation", "result"}),
		VerifyResults:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "golfs_verify_results_total", Help: "Upload verification outcomes."}, []string{"result"}),
	}
	registry.MustRegister(
		m.HTTPRequests, m.HTTPDuration, m.AuthCache,
		m.GitHubRequests, m.GitHubDuration,
		m.S3Requests, m.S3Duration,
		m.BatchObjects, m.VerifyResults,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "golfs_build_info",
			Help:        "Build information for GOLFS.",
			ConstLabels: prometheus.Labels{"version": version, "commit": commit},
		}, func() float64 { return 1 }),
	)
	return m
}

// Instrument records status and duration for a fixed low-cardinality route label.
func (m *Metrics) Instrument(listener, route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		m.HTTPRequests.WithLabelValues(listener, route, r.Method, strconv.Itoa(recorder.status)).Inc()
		m.HTTPDuration.WithLabelValues(listener, route, r.Method).Observe(time.Since(started).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
