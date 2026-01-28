package repl

import (
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// RREPL implements an R language REPL.
type RREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
}

// NewRREPL creates a new R REPL process.
func NewRREPL(id string, config process.ProcessConfig) (*RREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &RREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
	}, nil
}

// Start starts the R REPL process.
func (r *RREPL) Start() error {
	if r.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := r.GetConfig()

	// Try R interpreters
	rCandidates := []execCandidate{
		{"R", []string{"--interactive", "--quiet", "--no-save", "--no-restore"}},
		{"Rscript", []string{"--vanilla"}},
	}

	return startWithCandidates(r.BaseProcess, r.runner, config, rCandidates, nil)
}

// Stop stops the R REPL process.
func (r *RREPL) Stop() error {
	return r.runner.Stop()
}

// Restart restarts the process.
func (r *RREPL) Restart() error {
	if err := r.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return r.Start()
}

// ResizeTerminal resizes the PTY.
func (r *RREPL) ResizeTerminal(size process.PTYSize) error {
	if !r.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return r.BaseProcess.ResizePTY(size)
}
