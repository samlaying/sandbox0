package repl

import (
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// PHPREPL implements a PHP REPL.
type PHPREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
}

// NewPHPREPL creates a new PHP REPL process.
func NewPHPREPL(id string, config process.ProcessConfig) (*PHPREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &PHPREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
	}, nil
}

// Start starts the PHP REPL process.
func (p *PHPREPL) Start() error {
	if p.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := p.GetConfig()

	phpCandidates := []execCandidate{
		{"php", []string{"-a"}},
	}
	return startWithCandidates(p.BaseProcess, p.runner, config, phpCandidates, nil)
}

// Stop stops the PHP REPL process.
func (p *PHPREPL) Stop() error {
	return p.runner.Stop()
}

// Restart restarts the process.
func (p *PHPREPL) Restart() error {
	if err := p.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return p.Start()
}

// ResizeTerminal resizes the PTY.
func (p *PHPREPL) ResizeTerminal(size process.PTYSize) error {
	if !p.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return p.BaseProcess.ResizePTY(size)
}
