package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/mackerelio/mackerel-agent/checks"
	"github.com/mackerelio/mackerel-agent/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/pyama86/mackerel-check-plugin-exporter/mackerel"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	namespace = "mackerel-checks"
)

var (
	mackerelChecks = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "check_status"),
		"Status of health checks associated with mackerel checks.",
		[]string{"check_name", "status", "message"}, nil,
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
	conf   *config.Config
}

func NewExporter(logger log.Logger, conf *config.Config) (*Exporter, error) {
	return &Exporter{
		logger: logger,
		conf:   conf,
	}, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- mackerelChecks
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	for n, _ := range e.conf.CheckPlugins {
		v, ok := mackerel.CheckResult.Load(n)
		vm, _ := mackerel.CheckResultMessage.Load(n)
		if ok {

			var up float64
			switch v.(checks.Status) {
			case checks.StatusOK:
				up = 1.0
			case checks.StatusWarning:
				up = 2.0
			case checks.StatusCritical:
				up = 3.0
			case checks.StatusUnknown:
				up = 0.0
			}

			ch <- prometheus.MustNewConstMetric(
				mackerelChecks, prometheus.GaugeValue, up, n, string(v.(checks.Status)), vm.(string),
			)
		}

	}
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

	checkers := mackerel.CreateCheckers(mackerelConf)

	c := make(chan os.Signal, 1)
	termCh := make(chan struct{})
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)

	go signalHandler(c, termCh)
	go mackerel.Loop(checkers, termCh)

	exporter, err := NewExporter(logger, mackerelConf)
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

var maxTerminatingInterval = 30 * time.Second

func signalHandler(c chan os.Signal, termCh chan struct{}) {
	received := false
	for _ = range c {
		if !received {
			received = true
		}
		termCh <- struct{}{}
		go func() {
			time.Sleep(maxTerminatingInterval)
			termCh <- struct{}{}
		}()
	}
}
