// Package repl provides REPL process implementations.
package repl

import (
	"bytes"
	"strings"
	"sync"
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// PythonREPL implements a Python REPL using IPython.
type PythonREPL struct {
	*process.BaseProcess
	runner    *process.PTYRunner
	promptMu  sync.Mutex
	lastInput string
}

// NewPythonREPL creates a new Python REPL process.
func NewPythonREPL(id string, config process.ProcessConfig) (*PythonREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	repl := &PythonREPL{
		BaseProcess: bp,
	}
	repl.runner = process.NewPTYRunner(bp, repl.filterOutput, nil)
	return repl, nil
}

// Start starts the Python REPL process.
func (p *PythonREPL) Start() error {
	if p.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := p.GetConfig()

	// Try Python interpreters in order of preference:
	// 1. ipython - best interactive experience
	// 2. python3 - modern Python 3.x
	// 3. python - usually points to default Python
	// 4. python2 - legacy Python 2.x (for compatibility)
	pythonCandidates := []execCandidate{
		{"ipython3", []string{"--simple-prompt", "-i", "--no-banner", "--colors=NoColor"}},
		{"python3", []string{"-i", "-u"}},
		{"python", []string{"-i", "-u"}},
		{"python2", []string{"-i", "-u"}},
	}
	return startWithCandidates(p.BaseProcess, p.runner, config, pythonCandidates, []string{"PYTHONUNBUFFERED=1"})
}

// Stop stops the Python REPL process.
func (p *PythonREPL) Stop() error {
	return p.runner.Stop()
}

// Restart restarts the process.
func (p *PythonREPL) Restart() error {
	if err := p.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return p.Start()
}

// ResizeTerminal resizes the PTY.
func (p *PythonREPL) ResizeTerminal(size process.PTYSize) error {
	if !p.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return p.BaseProcess.ResizePTY(size)
}

func (p *PythonREPL) filterOutput(data []byte) []byte {
	p.promptMu.Lock()
	lastInput := p.lastInput
	p.promptMu.Unlock()

	// Remove echo of the last input command
	if lastInput != "" && bytes.Contains(data, []byte(lastInput)) {
		data = bytes.Replace(data, []byte(lastInput+"\n"), []byte{}, 1)
		data = bytes.Replace(data, []byte(lastInput+"\r\n"), []byte{}, 1)
	}

	return data
}

// detectPrompt checks if the output contains a Python prompt.
func (p *PythonREPL) detectPrompt(data []byte) bool {
	patterns := []string{
		"In [", // IPython input prompt
		"Out[", // IPython output prompt
		"...:", // Continuation prompt
		">>> ", // Standard Python prompt
		"... ", // Standard continuation
	}

	str := string(data)
	for _, pattern := range patterns {
		if strings.Contains(str, pattern) {
			return true
		}
	}
	return false
}
