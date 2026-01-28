package repl

import (
	"fmt"
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// ZshREPL implements a Zsh shell REPL.
type ZshREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
	prompt string
}

// NewZshREPL creates a new Zsh REPL process.
func NewZshREPL(id string, config process.ProcessConfig) (*ZshREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &ZshREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
		prompt:      "SANDBOX0>>> ",
	}, nil
}

// Start starts the Zsh REPL process.
func (z *ZshREPL) Start() error {
	if z.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := z.GetConfig()

	zshCandidates := []execCandidate{
		{"zsh", []string{"--no-rcs", "-i"}},
		{"bash", []string{"--norc", "--noprofile", "-i"}},
		{"sh", []string{"-i"}},
	}

	term := config.Term
	if term == "" {
		term = "xterm-256color"
	}
	extraEnv := []string{
		fmt.Sprintf("TERM=%s", term),
		fmt.Sprintf("PS1=%s", z.prompt),
	}
	return startWithCandidates(z.BaseProcess, z.runner, config, zshCandidates, extraEnv)
}

// Stop stops the Zsh REPL process.
func (z *ZshREPL) Stop() error {
	return z.runner.Stop()
}

// Restart restarts the process.
func (z *ZshREPL) Restart() error {
	if err := z.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return z.Start()
}

// ResizeTerminal resizes the PTY.
func (z *ZshREPL) ResizeTerminal(size process.PTYSize) error {
	if !z.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return z.BaseProcess.ResizePTY(size)
}
