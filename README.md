# Icecast exporter for Prometheus

This is a simple server that periodically scrapes stats from the Icecast streaming media server
and exports them via HTTP/JSON for Prometheus consumption. It requires the JSON API (`/status-json.xsl`)
provided by Icecast 2.4.0 or newer.

To run it:

```bash
go run icecast_exporter [flags]
```

Help on flags:
```bash
go run icecast_exporter --help
```