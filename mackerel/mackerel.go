package mackerel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mackerelio/golib/logging"
	"github.com/mackerelio/mackerel-agent/checks"
	"github.com/mackerelio/mackerel-agent/config"
)

var (
	reportCheckDelaySeconds    = 1      // Wait for a second before reporting the next check
	reportCheckDelaySecondsMax = 30     // Wait 30 seconds before reporting the next check when many reports in queue
	reportCheckBufferSize      = 6 * 60 // Keep check reports of 6 hours in the queue
)
var logger = logging.GetLogger("command")

var CheckResult sync.Map
var CheckResultMessage sync.Map

func init() {
	CheckResult = sync.Map{}
	CheckResultMessage = sync.Map{}
}

// LICENCE: https://github.com/mackerelio/mackerel-agent/blob/master/LICENSE
// import from github.com/mackerelio/mackerel-agent/command/command.go
func CreateCheckers(conf *config.Config) []*checks.Checker {
	checkers := []*checks.Checker{}

	for name, pluginConfig := range conf.CheckPlugins {
		checker := &checks.Checker{
			Name:   name,
			Config: pluginConfig,
		}
		checkers = append(checkers, checker)
	}

	return checkers
}

func Loop(checkers []*checks.Checker, termCh chan struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var termCheckerCh chan struct{}
	hasChecks := len(checkers) > 0
	if hasChecks {
		termCheckerCh = make(chan struct{})
	}

	// fan-out termCh
	go func() {
		for range termCh {
			if termCheckerCh != nil {
				termCheckerCh <- struct{}{}
			}
		}
	}()

	if hasChecks {
		runCheckersLoop(ctx, checkers, termCheckerCh)
	}
	return nil
}
func runCheckersLoop(ctx context.Context, checkers []*checks.Checker, termCheckerCh <-chan struct{}) {
	// Do not block checking.
	checkReportCh := make(chan *checks.Report, reportCheckBufferSize*len(checkers))
	reportImmediateCh := make(chan struct{}, reportCheckBufferSize*len(checkers))

	for _, checker := range checkers {
		go runChecker(ctx, checker, checkReportCh, reportImmediateCh)
	}

	for {
		select {
		case <-termCheckerCh:
			logger.Debugf("received 'term' chan for checkers loop")
			return
		case report := <-checkReportCh:
			CheckResult.Store(report.Name, report.Status)
			CheckResultMessage.Store(report.Name, report.Message)
		}
	}

}

func runChecker(ctx context.Context, checker *checks.Checker, checkReportCh chan *checks.Report, reportImmediateCh chan struct{}) {
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
					logger.Debugf("Checker %q action: %q env: %+v", checker.Name, checker.Config.Action.CommandString(), env)
					stdout, stderr, exitCode, _ := checker.Config.Action.RunWithEnv(env)

					if stderr != "" {
						logger.Warningf("Checker %q action stdout: %q stderr: %q exitCode: %d", checker.Name, stdout, stderr, exitCode)
					} else {
						logger.Debugf("Checker %q action stdout: %q exitCode: %d", checker.Name, stdout, exitCode)
					}
				}()
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

			// If status has changed, send it immediately
			// but if the status was OK and it's first invocation of a check, do not
			if report.Status != lastStatus && !(report.Status == checks.StatusOK && lastStatus == checks.StatusUndefined) {
				logger.Debugf("checker %q: status has changed %v -> %v: send it immediately", checker.Name, lastStatus, report.Status)
				reportImmediateCh <- struct{}{}
			}

			lastStatus = report.Status
			lastMessage = report.Message
		case <-ctx.Done():
			return
		}
	}
}
