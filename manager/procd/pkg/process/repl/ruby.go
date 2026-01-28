package repl

import (
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// RubyREPL implements a Ruby REPL using IRB.
type RubyREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
}

// NewRubyREPL creates a new Ruby REPL process.
func NewRubyREPL(id string, config process.ProcessConfig) (*RubyREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &RubyREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
	}, nil
}

// Start starts the Ruby REPL process.
func (r *RubyREPL) Start() error {
	if r.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := r.GetConfig()

	// Try Ruby interpreters in order of preference
	rubyCandidates := []execCandidate{
		{"irb", []string{"--simple-prompt", "--noreadline"}},
		{"ruby", []string{"-e", "require 'irb'; IRB.start"}},
	}

	return startWithCandidates(r.BaseProcess, r.runner, config, rubyCandidates, nil)
}

// Stop stops the Ruby REPL process.
func (r *RubyREPL) Stop() error {
	return r.runner.Stop()
}

// Restart restarts the process.
func (r *RubyREPL) Restart() error {
	if err := r.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return r.Start()
}

// ResizeTerminal resizes the PTY.
func (r *RubyREPL) ResizeTerminal(size process.PTYSize) error {
	if !r.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return r.BaseProcess.ResizePTY(size)
}
