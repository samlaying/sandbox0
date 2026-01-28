package repl

import (
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// LuaREPL implements a Lua REPL.
type LuaREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
}

// NewLuaREPL creates a new Lua REPL process.
func NewLuaREPL(id string, config process.ProcessConfig) (*LuaREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &LuaREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
	}, nil
}

// Start starts the Lua REPL process.
func (l *LuaREPL) Start() error {
	if l.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := l.GetConfig()

	// Try Lua interpreters in order of preference
	luaCandidates := []execCandidate{
		{"lua", []string{"-i"}},
		{"lua5.4", []string{"-i"}},
		{"lua5.3", []string{"-i"}},
		{"lua5.2", []string{"-i"}},
		{"lua5.1", []string{"-i"}},
		{"luajit", []string{"-i"}},
	}

	return startWithCandidates(l.BaseProcess, l.runner, config, luaCandidates, nil)
}

// Stop stops the Lua REPL process.
func (l *LuaREPL) Stop() error {
	return l.runner.Stop()
}

// Restart restarts the process.
func (l *LuaREPL) Restart() error {
	if err := l.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return l.Start()
}

// ResizeTerminal resizes the PTY.
func (l *LuaREPL) ResizeTerminal(size process.PTYSize) error {
	if !l.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return l.BaseProcess.ResizePTY(size)
}
