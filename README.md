# stat-exporter

This is a miniscule project that exports data from `/proc/stat` to Prometheus. The aim of this project was to depict 
the ease of implementing the `prometheus.Collector` interface in order to create a collector that exports data to a 
prometheus-compatible endpoint.

## Usage

```bash
$ go install github.com/rexagod/stat-exporter@latest
$ stat-exporter # [--port=":3000"] (default port is :8080)
$ curl localhost:8080/metrics # or whatever port you specified
```

## License

[GNU AGPLv3](https://www.gnu.org/licenses/agpl-3.0.en.html)
