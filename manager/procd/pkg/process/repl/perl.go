package repl

import (
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// PerlREPL implements a Perl REPL.
type PerlREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
}

// NewPerlREPL creates a new Perl REPL process.
func NewPerlREPL(id string, config process.ProcessConfig) (*PerlREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &PerlREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
	}, nil
}

// Start starts the Perl REPL process.
func (p *PerlREPL) Start() error {
	if p.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := p.GetConfig()

	// Try Perl interpreters in order of preference
	perlCandidates := []execCandidate{
		{"re.pl", []string{}},          // Perl REPL if installed via cpanm Devel::REPL
		{"perl", []string{"-de", "0"}}, // Perl debugger as REPL
	}

	return startWithCandidates(p.BaseProcess, p.runner, config, perlCandidates, nil)
}

// Stop stops the Perl REPL process.
func (p *PerlREPL) Stop() error {
	return p.runner.Stop()
}

// Restart restarts the process.
func (p *PerlREPL) Restart() error {
	if err := p.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return p.Start()
}

// ResizeTerminal resizes the PTY.
func (p *PerlREPL) ResizeTerminal(size process.PTYSize) error {
	if !p.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return p.BaseProcess.ResizePTY(size)
}
