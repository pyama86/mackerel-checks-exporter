package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"git.pepabo.com/fukuoka-admin/ghost/config"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/k0kubun/pp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	namespace = "mackerel-checks"
)

var (
	checks = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "check_status"),
		"Status of health checks associated with mackerel checks.",
		[]string{"check", "node", "check_name", "status"}, nil,
	)
)

type promHTTPLogger struct {
	logger log.Logger
}

func (l promHTTPLogger) Println(v ...interface{}) {
	level.Error(l.logger).Log("msg", fmt.Sprint(v...))
}

type Exporter struct {
	logger log.Logger
}

func NewExporter(logger log.Logger) (*Exporter, error) {
	return &Exporter{
		logger: logger,
	}, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- checks
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	//	ok := e.collectPeersMetric(ch)
	/*
		if ok {
			ch <- prometheus.MustNewConstMetric(
				up, prometheus.GaugeValue, 1.0,
			)
		} else {
			ch <- prometheus.MustNewConstMetric(
				up, prometheus.GaugeValue, 0.0,
			)
		}
	*/
}

func init() {
	prometheus.MustRegister(version.NewCollector("mackerel-chekcs"))
}

func main() {
	var (
		listenAddress      = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9111").String()
		metricsPath        = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		mackerelConfigPath = kingpin.Flag("mackerel.config-path", "Mackerel Config Path.").Default("/etc/mackerel-agent/mackerel-agent.conf").String()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting mackerel_checks_exporter", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	mackerelConf, err := config.LoadConfig(*mackerelConfigPath)

	pp.Println(mackerelConf)
	if err != nil {
		level.Error(logger).Log("msg", "can't read mackerell config", "err", err)
		os.Exit(1)
	}
	exporter, err := NewExporter(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error creating the exporter", "err", err)
		os.Exit(1)
	}
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath,
		promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer,
			promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{
					ErrorLog: &promHTTPLogger{
						logger: logger,
					},
				},
			),
		),
	)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Mackerel Checks Exporter</title></head>
             <body>
             <h1>Mackerel Checks Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             <h2>Build</h2>
             <pre>` + version.Info() + ` ` + version.BuildContext() + `</pre>
             </body>
             </html>`))
	})

	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}
}
