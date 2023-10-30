package mackerel

import (
	"context"
	"testing"
	"time"

	"github.com/mackerelio/mackerel-agent/cmdutil"
	"github.com/mackerelio/mackerel-agent/config"

	"github.com/mackerelio/golib/logging"
	"github.com/mackerelio/mackerel-agent/checks"
)

func TestLoop(t *testing.T) {
	if testing.Verbose() {
		logging.SetLogLevel(logging.DEBUG)
	}

	ctx, cancel := context.WithCancel(context.Background())
	exitCh := make(chan error)

	checkers := []*checks.Checker{
		&checks.Checker{
			Name: "example1",
			Config: &config.CheckPlugin{
				Command: config.Command{
					Args: []string{"echo", "0"},
				},
			},
		},
	}

	// Start looping!
	go func() {
		exitCh <- Loop(checkers, nil, ctx)
	}()

	timer := time.NewTimer(time.Second * 1)
	<-timer.C

	v, _ := CheckResult.Load("example1")
	if v.(checks.Status) != "OK" {
		t.Errorf("can't get status got: %s", v)
	}

	cancel()
	exitErr := <-exitCh
	if exitErr != nil {
		t.Errorf("exitErr should be nil, got: %s", exitErr)
	}
}

func TestRunChecker(t *testing.T) {
	tests := []struct {
		name         string
		maxCheck     int32
		action       *config.Command
		expectStatus checks.Status
	}{
		{
			name:     "test case with success action",
			maxCheck: 0,
			action: &config.Command{
				Cmd: "true",
				CommandOption: cmdutil.CommandOption{
					Env:             []string{},
					TimeoutDuration: time.Second * 5,
				},
			},
			expectStatus: checks.StatusOK,
		},
		{
			name:     "test case with error action",
			maxCheck: 0,
			action: &config.Command{
				Cmd: "false",
				CommandOption: cmdutil.CommandOption{
					Env:             []string{},
					TimeoutDuration: time.Second * 5,
				},
			},

			expectStatus: checks.StatusWarning,
		},
		{
			name:     "test case with error action and maxCheck",
			maxCheck: 1,
			action: &config.Command{
				Cmd: "false",
				CommandOption: cmdutil.CommandOption{
					Env:             []string{},
					TimeoutDuration: time.Second * 5,
				},
			},

			expectStatus: checks.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			checkReportCh := make(chan *checks.Report, 1)
			checker := &checks.Checker{
				Name: tt.name,
				Config: &config.CheckPlugin{
					Action:                tt.action,
					Command:               *tt.action,
					MaxCheckAttempts:      &tt.maxCheck,
					PreventAlertAutoClose: false,
				},
			}

			go runChecker(ctx, checker, checkReportCh)

			select {
			case report := <-checkReportCh:
				if report.Status != tt.expectStatus {
					t.Errorf("expected status to be %s, but got %s", tt.expectStatus, report.Status)
				}
			case <-ctx.Done():
				t.Error("context deadline exceeded while waiting for the checker result")
			}
		})
	}
}
