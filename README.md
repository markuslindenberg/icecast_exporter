# Icecast exporter for Prometheus

This is a simple [Prometheus](https://prometheus.io/) exporter that scrapes stats from the [Icecast](http://icecast.org/) streaming media server. It requires the JSON API (`/status-json.xsl`)
provided by Icecast 2.4.0 or newer.

By default icecast_exporter listens on port 9146 for HTTP requests.

## Installation

### Using `go get`

```bash
go get github.com/markuslindenberg/icecast_exporter
```
### Using Docker

```
docker pull markuslindenberg/icecast_exporter
docker run --rm -p 9146:9146 markuslindenberg/icecast_exporter -icecast.scrape-uri http://icecast:8000/status-json.xsl
```

# Running

Help on flags:
```
go run icecast_exporter --help

Usage of ./icecast_exporter:
  -icecast.scrape-uri string
    	URI on which to scrape Icecast. (default "http://localhost:8000/status-json.xsl")
  -icecast.time-format
      Time format used by Icecast. (defautl "2006-01-02T15:04:05-0700")
  -icecast.timeout duration
    	Timeout for trying to get stats from Icecast. (default 5s)
  -log.format value
    	Set the log target and format. Example: "logger:syslog?appname=bob&local=7" or "logger:stdout?json=true" (default "logger:stderr")
  -log.level value
    	Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]
  -web.listen-address string
    	Address to listen on for web interface and telemetry. (default ":9146")
  -web.telemetry-path string
    	Path under which to expose metrics. (default "/metrics")
```
