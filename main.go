package main

import (
	"bufio"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
)

// collector implements the prometheus.Collector interface.
type collector struct{}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {}

func (c *collector) Collect(ch chan<- prometheus.Metric) {}

type cpu []struct {
	// core number
	id uint64
	// normal processes executing in user mode
	user uint64
	// niced processes executing in user mode
	nice uint64
	// processes executing in kernel mode
	system uint64
	// twiddling thumbs
	idle uint64
	// waiting for I/O to complete
	iowait uint64
	// servicing interrupts
	irq uint64
	// servicing softirqs
	softirq uint64
}

// stat exports non-cpu members so that they can be reflected dynamically.
// "/proc/stat" has these members specified textually, and hence can be
// used for the same, as opposed to the cores' stats.
type stat struct {
	cpu
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

func (s *stat) parse(r io.Reader) {
	defer func() {
		rec := recover()
		if rec != nil {
			klog.ErrorS(nil, "Failed to parse stat file", "error", rec)
		}
	}()
	scanner := bufio.NewScanner(r)
	// Skip over aggregated cpu stats.
	scanner.Scan()
	// Fetch all cores' info.
	parseUintX := func(s string) uint64 {
		n, _ := strconv.ParseUint(s, 10, 64)
		return n
	}
	var coreStat cpu
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		fields := strings.Fields(line)
		if strings.HasPrefix(line, "cpu") {
			coreStat = append(coreStat, struct {
				id      uint64
				user    uint64
				nice    uint64
				system  uint64
				idle    uint64
				iowait  uint64
				irq     uint64
				softirq uint64
			}{
				id:      parseUintX(fields[1]),
				user:    parseUintX(fields[2]),
				nice:    parseUintX(fields[3]),
				system:  parseUintX(fields[4]),
				idle:    parseUintX(fields[5]),
				iowait:  parseUintX(fields[6]),
				irq:     parseUintX(fields[7]),
				softirq: parseUintX(fields[8]),
			},
			)
		} else {
			break
		}
	}
	s.cpu = coreStat
	// Fetch other info.
	for scanner.Scan() {
		fields := strings.Fields(line)
		infoType := strings.ToUpper(string(fields[0][0])) + fields[0][1:]
		infoValue := parseUintX(fields[1])
		reflect.ValueOf(s).Elem().FieldByName(infoType).SetUint(infoValue)
		// Populate the struct dynamically.
		line = scanner.Text()
	}
}

func main() {
	var overallStats stat
	dat, err := os.ReadFile("/proc/stat")
	if err != nil {
		klog.ErrorS(err, "Failed to read stat file")
	}
	overallStats.parse(strings.NewReader(string(dat)))

	//mux := http.NewServeMux()
	//mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	//
	//err := http.ListenAndServe(":8080", mux)
	//if err != nil {
	//	klog.ErrorS(err, "Failed to start metrics server")
	//	klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	//}
}
