package repl

import (
	"fmt"
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// BashREPL implements a Bash shell REPL.
type BashREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
	prompt string
}

// NewBashREPL creates a new Bash REPL process.
func NewBashREPL(id string, config process.ProcessConfig) (*BashREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &BashREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
		prompt:      "SANDBOX0>>> ",
	}, nil
}

// Start starts the Bash REPL process.
func (b *BashREPL) Start() error {
	if b.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := b.GetConfig()

	bashCandidates := []execCandidate{
		{"bash", []string{"--norc", "--noprofile", "-i"}},
		{"sh", []string{"-i"}},
	}

	// Set TERM
	term := config.Term
	if term == "" {
		term = "xterm-256color"
	}

	extraEnv := []string{
		fmt.Sprintf("TERM=%s", term),
		fmt.Sprintf("PS1=%s", b.prompt),
	}
	return startWithCandidates(b.BaseProcess, b.runner, config, bashCandidates, extraEnv)
}

// Stop stops the Bash REPL process.
func (b *BashREPL) Stop() error {
	return b.runner.Stop()
}

// Restart restarts the process.
func (b *BashREPL) Restart() error {
	if err := b.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return b.Start()
}

// ResizeTerminal resizes the PTY.
func (b *BashREPL) ResizeTerminal(size process.PTYSize) error {
	if !b.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return b.BaseProcess.ResizePTY(size)
}
