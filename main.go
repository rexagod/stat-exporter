package main

import (
	"bufio"
	"flag"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

type core struct {

	// core number
	Id uint64

	// normal processes executing in user mode
	User uint64

	// niced processes executing in user mode
	Nice uint64

	// processes executing in kernel mode
	System uint64

	// twiddling thumbs
	Idle uint64

	// waiting for I/O to complete
	Iowait uint64

	// servicing interrupts
	Irq uint64

	// servicing softirqs
	Softirq uint64
}

// stats exports non-core members so that they can be reflected dynamically.
// "/proc/stat" has these members specified textually, and hence can be
// used for the same, as opposed to the cores' stats.
type stats struct {
	Cores []core

	// total of all interrupts serviced since boot time
	Intr uint64

	// total number of context switches across all CPUs
	Ctxt uint64

	// time at which the system booted, in seconds since the Unix epoch
	Btime uint64

	// number of processes and threads created
	Processes uint64

	// number of processes currently running
	Procs_running uint64

	// number of processes currently blocked, waiting for I/O to complete
	Procs_blocked uint64
}

func (s *stats) parse(r io.Reader) {
	defer func() {
		rec := recover()
		if rec != nil {
			klog.ErrorS(nil, "Failed to parse stat file.", "error", rec)
		}
	}()
	scanner := bufio.NewScanner(r)

	// Skip over aggregated core stats.
	scanner.Scan()

	// Fetch all cores' info.
	parseUintX := func(s string) uint64 {
		n, _ := strconv.ParseUint(s, 10, 64)
		return n
	}
	var coreStat []core
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		fields := strings.Fields(line)
		if strings.HasPrefix(line, "cpu") {
			singleStat := core{
				Id:      parseUintX(fields[1]),
				User:    parseUintX(fields[2]),
				Nice:    parseUintX(fields[3]),
				System:  parseUintX(fields[4]),
				Idle:    parseUintX(fields[5]),
				Iowait:  parseUintX(fields[6]),
				Irq:     parseUintX(fields[7]),
				Softirq: parseUintX(fields[8]),
			}
			coreStat = append(coreStat, singleStat)
		} else {
			break
		}
	}
	s.Cores = coreStat

	// Fetch other info.
	for scanner.Scan() {
		fields := strings.Fields(line)
		infoType := strings.ToUpper(string(fields[0][0])) + fields[0][1:]
		infoValue := parseUintX(fields[1])

		// Populate the struct dynamically.
		reflect.ValueOf(s).Elem().FieldByName(infoType).SetUint(infoValue)
		line = scanner.Text()
	}
}

// collector implements the prometheus.Collector interface.
type collector struct {
	s *stats
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	dat, err := os.ReadFile("/proc/stat")
	if err != nil {
		klog.ErrorS(err, "Failed to read /proc/stat.")
	}

	// Populate stats.
	c.s.parse(strings.NewReader(string(dat)))

	// Collect metrics.
	var metricName string
	var metricValue uint64
	r := reflect.ValueOf(c.s).Elem()

	// Collect cores' stats.
	metricName = r.Type().Field(0).Name
	metricName = strings.ToLower(string(metricName[0])) + metricName[1:]
	for coreCount, singleCore := range c.s.Cores {
		reflectedCore := reflect.ValueOf(singleCore)
		for i := 0; i < reflectedCore.NumField(); i++ {
			metricName = reflectedCore.Type().Field(i).Name
			metricName = strings.ToLower(string(metricName[0])) + metricName[1:]
			metricValue = reflectedCore.Field(i).Uint()
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(
						"",
						"core"+strconv.Itoa(coreCount),
						metricName,
					),
					metricName+" info",
					nil,
					nil,
				),
				prometheus.GaugeValue,
				float64(metricValue),
			)
		}
	}

	// Miscellaneous information.
	for i := 1; i < r.NumField(); i++ {
		metricName = r.Type().Field(i).Name
		metricName = strings.ToLower(string(metricName[0])) + metricName[1:]
		metricValue = r.Field(i).Uint()
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(
					"",
					"fs",
					metricName,
				),
				metricName+" info",
				nil,
				nil,
			),
			prometheus.GaugeValue,
			float64(metricValue),
		)
	}
}

func main() {
	// Initialize the flags.
	//metricsPrefix := flag.String("metrics-prefix", "sys", "Prefix for the metrics.")
	port := flag.String("port", ":8080", "Port to listen on.")
	flag.Parse()

	// Register the collector.
	reg := prometheus.NewPedanticRegistry()
	c := &collector{s: &stats{}}
	//prometheus.WrapRegistererWithPrefix(*metricsPrefix+"_", reg).MustRegister(c)
	prometheus.WrapRegistererWith(prometheus.Labels{"domain": "fs"}, reg).MustRegister(c)

	// Register in-built process collector for comparison.
	//reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Register in-built collector for debug metrics.
	//reg.MustRegister(collectors.NewGoCollector())

	// Handle metrics endpoint.
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", promhttp.InstrumentMetricHandler(reg, promhttp.HandlerFor(reg, promhttp.HandlerOpts{})).ServeHTTP)

	// Start the server.
	klog.InfoS("Starting metrics server.", "port", *port)
	err := http.ListenAndServe(*port, mux)
	if err != nil {
		klog.ErrorS(err, "Failed to start metrics server.")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}
