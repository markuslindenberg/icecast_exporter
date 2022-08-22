// Copyright 2016 Markus Lindenberg
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	namespace = "icecast"
)

var (
	labelNames = []string{"listenurl", "server_type"}
)

type ISO8601 time.Time

func (ts ISO8601) Time() time.Time {
	return time.Time(ts)
}

func (ts *ISO8601) UnmarshalJSON(data []byte) error {
	parsed, err := time.Parse(`"2006-01-02T15:04:05-0700"`, string(data))
	if err != nil {
		return err
	}
	*ts = ISO8601(parsed)
	return nil
}

type IcecastStatusSource struct {
	Listeners   int     `json:"listeners"`
	Listenurl   string  `json:"listenurl"`
	ServerType  string  `json:"server_type"`
	StreamStart ISO8601 `json:"stream_start_iso8601"`
}

// JSON structure if zero or multiple streams active
type IcecastStatus struct {
	Icestats struct {
		ServerStart ISO8601					`json:"server_start_iso8601"`
		Source      []IcecastStatusSource 	`json:"source,omitifempty"`
	} `json:"icestats"`
}

// JSON structure if exactly one stream active
type IcecastStatusSingle struct {
	Icestats struct {
		ServerStart ISO8601 				`json:"server_start_iso8601"`
		Source      IcecastStatusSource 	`json:"source"`
	} `json:"icestats"`
}


// Exporter collects Icecast stats from the given URI and exports them using
// the prometheus metrics package.
type Exporter struct {
	URI   string
	mutex sync.RWMutex

	up                              prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter
	serverStart                     prometheus.Gauge
	listeners                       *prometheus.GaugeVec
	streamStart                     *prometheus.GaugeVec
	client                          *http.Client
}

// NewExporter returns an initialized Exporter.
func NewExporter(uri string, timeout time.Duration) *Exporter {
	return &Exporter{
		URI: uri,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the last scrape of Icecast successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_total_scrapes",
			Help:      "Current total Icecast scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_json_parse_failures",
			Help:      "Number of errors while parsing JSON.",
		}),
		serverStart: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_start",
			Help:      "Timestamp of server startup.",
		}),
		listeners: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "listeners",
			Help:      "The number of currently connected listeners.",
		}, labelNames),
		streamStart: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "stream_start",
			Help:      "Timestamp of when the currently active source client connected to this mount point.",
		}, labelNames),
		client: &http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					c, err := net.DialTimeout(netw, addr, timeout)
					if err != nil {
						return nil, err
					}
					if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
						return nil, err
					}
					return c, nil
				},
			},
		},
	}
}

// Describe describes all the metrics ever exported by the Icecast exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up.Desc()
	ch <- e.totalScrapes.Desc()
	ch <- e.jsonParseFailures.Desc()
	ch <- e.serverStart.Desc()
	e.listeners.Describe(ch)
	e.streamStart.Describe(ch)
}

// Collect fetches the stats from configured Icecast location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	status := make(chan *IcecastStatus)
	go e.scrape(status)

	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	e.listeners.Reset()
	e.streamStart.Reset()

	if s := <-status; s != nil {
		e.serverStart.Set(float64(s.Icestats.ServerStart.Time().Unix()))
		for _, source := range s.Icestats.Source {
			e.listeners.WithLabelValues(source.Listenurl, source.ServerType).Set(float64(source.Listeners))
			e.streamStart.WithLabelValues(source.Listenurl, source.ServerType).Set(float64(source.StreamStart.Time().Unix()))
		}
	}

	ch <- e.up
	ch <- e.totalScrapes
	ch <- e.jsonParseFailures
	ch <- e.serverStart
	e.listeners.Collect(ch)
	e.streamStart.Collect(ch)
}

func (e *Exporter) scrape(status chan<- *IcecastStatus) {
	defer close(status)

	e.totalScrapes.Inc()

	resp, err := e.client.Get(e.URI)
	if err != nil {
		e.up.Set(0)
		log.Errorf("Can't scrape Icecast: %v", err)
		return
	}
	defer resp.Body.Close()
	e.up.Set(1)
	
	// Copy response body into intermediate buffer,
	// so we can deserialize twice
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		e.up.Set(0)
		log.Errorf("Can't ready response body: %v", err)
		return
	}
	
	buf := bytes.NewBuffer(bodyBytes)
	var s IcecastStatus
	err = json.NewDecoder(buf).Decode(&s)

	if err != nil {
		// If only a single stream is active, the JSON will
		// have a different format with "source" being an object
		buf := bytes.NewBuffer(bodyBytes)
		var s2 IcecastStatusSingle
		err = json.NewDecoder(buf).Decode(&s2)
		if err != nil {
			log.Errorf("Can't read JSON: %v", err)
			e.jsonParseFailures.Inc()
			return
		}
		
		// Copy over to staus object
		s.Icestats.ServerStart = s2.Icestats.ServerStart
		s.Icestats.Source = []IcecastStatusSource{s2.Icestats.Source}
	}

	status <- &s
}

func main() {
	var (
		listenAddress    = flag.String("web.listen-address", ":9146", "Address to listen on for web interface and telemetry.")
		metricsPath      = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		icecastScrapeURI = flag.String("icecast.scrape-uri", "http://localhost:8000/status-json.xsl", "URI on which to scrape Icecast.")
		icecastTimeout   = flag.Duration("icecast.timeout", 5*time.Second, "Timeout for trying to get stats from Icecast.")
	)
	flag.Parse()

	// Listen to signals
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	exporter := NewExporter(*icecastScrapeURI, *icecastTimeout)
	prometheus.MustRegister(exporter)

	// Setup HTTP server
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Icecast Exporter</title></head>
             <body>
             <h1>Icecast Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	go func() {
		log.Infof("Starting Server: %s", *listenAddress)
		log.Fatal(http.ListenAndServe(*listenAddress, nil))
	}()

	s := <-sigchan
	log.Infof("Received %v, terminating", s)
	os.Exit(0)
}
