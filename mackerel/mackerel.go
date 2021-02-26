package mackerel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mackerelio/golib/logging"
	"github.com/mackerelio/mackerel-agent/checks"
	"github.com/mackerelio/mackerel-agent/config"
	"github.com/mackerelio/mackerel-agent/metrics"
)

var (
	reportCheckDelaySeconds    = 1      // Wait for a second before reporting the next check
	reportCheckDelaySecondsMax = 30     // Wait 30 seconds before reporting the next check when many reports in queue
	reportCheckBufferSize      = 6 * 60 // Keep check reports of 6 hours in the queue
	reportPluginInterval       = 60     // Wait 30 seconds before reporting the next check when many reports in queue
)
var logger = logging.GetLogger("command")

var CheckResult sync.Map
var CheckResultMessage sync.Map
var PluginResult sync.Map

func init() {
	CheckResult = sync.Map{}
	PluginResult = sync.Map{}
	CheckResultMessage = sync.Map{}
}

// LICENCE: https://github.com/mackerelio/mackerel-agent/blob/master/LICENSE
// import from github.com/mackerelio/mackerel-agent/command/command.go
func CreateCheckers(conf *config.Config) []*checks.Checker {
	checkers := []*checks.Checker{}
	if conf.CheckPlugins != nil {
		for name, pluginConfig := range conf.CheckPlugins {
			checker := &checks.Checker{
				Name:   name,
				Config: pluginConfig,
			}
			checkers = append(checkers, checker)
		}
	}

	return checkers
}

func PluginGenerators(conf *config.Config) []metrics.PluginGenerator {
	generators := []metrics.PluginGenerator{}
	for _, pluginConfig := range conf.MetricPlugins {
		generators = append(generators, metrics.NewPluginGenerator(pluginConfig))
	}

	if conf.Diagnostic {
		generators = append(generators, &metrics.AgentGenerator{})
	}
	return generators
}
func Loop(checkers []*checks.Checker, plugins []metrics.PluginGenerator, ctx context.Context) error {
	if len(checkers) > 0 {
		runCheckersLoop(ctx, checkers, plugins)
	}
	return nil
}
func runCheckersLoop(ctx context.Context, checkers []*checks.Checker, plugins []metrics.PluginGenerator) {
	// Do not block checking.
	checkReportCh := make(chan *checks.Report, reportCheckBufferSize*len(checkers))

	for _, checker := range checkers {
		go runChecker(ctx, checker, checkReportCh)
	}
	ticker := time.NewTicker(time.Second * time.Duration(reportPluginInterval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			values := generateValues(plugins)
			for _, vs := range values {
				if vs != nil {
					for k, v := range vs.Values {
						PluginResult.Store(k, v)
					}
				}
			}
		case report := <-checkReportCh:
			CheckResult.Store(report.Name, report.Status)
			CheckResultMessage.Store(report.Name, report.Message)
		}
	}

}

func runChecker(ctx context.Context, checker *checks.Checker, checkReportCh chan *checks.Report) {
	lastStatus := checks.StatusUndefined
	lastMessage := ""
	interval := checker.Interval()
	nextInterval := time.Duration(0)
	nextTime := time.Now()

	for {
		select {
		case <-time.After(nextInterval):
			report := checker.Check()
			logger.Debugf("checker %q: report=%v", checker.Name, report)

			now := time.Now()
			nextInterval = interval - (now.Sub(nextTime) % interval)
			nextTime = now.Add(nextInterval)

			if checker.Config.Action != nil {
				env := []string{fmt.Sprintf("MACKEREL_STATUS=%s", report.Status), fmt.Sprintf("MACKEREL_PREVIOUS_STATUS=%s", lastStatus), fmt.Sprintf("MACKEREL_CHECK_MESSAGE=%s", report.Message)}
				go func() {
					logger.Infof("Checker %q action: %q env: %+v", checker.Name, checker.Config.Action.CommandString(), env)
					stdout, stderr, exitCode, _ := checker.Config.Action.RunWithEnv(env)

					if stderr != "" {
						logger.Warningf("Checker %q action stdout: %q stderr: %q exitCode: %d", checker.Name, stdout, stderr, exitCode)
					} else {
						logger.Debugf("Checker %q action stdout: %q exitCode: %d", checker.Name, stdout, exitCode)
					}
				}()
			}

			if report.Status != checks.StatusOK && report.Status != lastStatus {
				logger.Infof("checker %s result:%s message=%s", checker.Name, report.Status, report.Message)
			}

			if report.Status == checks.StatusOK && report.Status == lastStatus && report.Message == lastMessage {
				// Do not report if nothing has changed
				continue
			}
			if report.Status == checks.StatusOK && checker.Config.PreventAlertAutoClose {
				// Do not report `OK` if `PreventAlertAutoClose`
				lastStatus = report.Status
				lastMessage = report.Message
				continue
			}
			checkReportCh <- report
			lastStatus = report.Status
			lastMessage = report.Message
		case <-ctx.Done():
			return
		}
	}
}

func generateValues(generators []metrics.PluginGenerator) []*metrics.ValuesCustomIdentifier {
	processed := make(chan *metrics.ValuesCustomIdentifier)
	finish := make(chan struct{})
	result := make(chan []*metrics.ValuesCustomIdentifier)

	go func() {
		allValues := []*metrics.ValuesCustomIdentifier{}
		for {
			select {
			case values := <-processed:
				allValues = metrics.MergeValuesCustomIdentifiers(allValues, values)
			case <-finish:
				result <- allValues
				return
			}
		}
	}()

	go func() {
		var wg sync.WaitGroup
		for _, g := range generators {
			wg.Add(1)
			go func(g metrics.Generator) {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("Panic: generating value in %T (skip this metric): %s", g, r)
					}
					wg.Done()
				}()

				startedAt := time.Now()
				values, err := g.Generate()
				if seconds := (time.Now().Sub(startedAt) / time.Second); seconds > 120 {
					logger.Warningf("%T.Generate() take a long time (%d seconds)", g, seconds)
				}
				if err != nil {
					logger.Errorf("Failed to generate value in %T (skip this metric): %s", g, err.Error())
					return
				}
				var customIdentifier *string
				if pluginGenerator, ok := g.(metrics.PluginGenerator); ok {
					customIdentifier = pluginGenerator.CustomIdentifier()
				}
				processed <- &metrics.ValuesCustomIdentifier{
					Values:           values,
					CustomIdentifier: customIdentifier,
				}
			}(g)
		}
		wg.Wait()
		finish <- struct{}{} // processed all jobs
	}()

	return <-result
}
