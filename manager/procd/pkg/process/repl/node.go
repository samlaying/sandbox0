package repl

import (
	"time"

	"github.com/sandbox0-ai/infra/manager/procd/pkg/process"
)

// NodeREPL implements a Node.js REPL.
type NodeREPL struct {
	*process.BaseProcess
	runner *process.PTYRunner
}

// NewNodeREPL creates a new Node.js REPL process.
func NewNodeREPL(id string, config process.ProcessConfig) (*NodeREPL, error) {
	bp := process.NewBaseProcess(id, process.ProcessTypeREPL, config)

	return &NodeREPL{
		BaseProcess: bp,
		runner:      process.NewPTYRunner(bp, nil, nil),
	}, nil
}

// Start starts the Node.js REPL process.
func (n *NodeREPL) Start() error {
	if n.IsRunning() {
		return process.ErrProcessAlreadyRunning
	}

	config := n.GetConfig()

	nodeCandidates := []execCandidate{
		{"node", []string{"--interactive"}},
		{"nodejs", []string{"--interactive"}},
	}
	return startWithCandidates(n.BaseProcess, n.runner, config, nodeCandidates, nil)
}

// Stop stops the Node.js REPL process.
func (n *NodeREPL) Stop() error {
	return n.runner.Stop()
}

// Restart restarts the process.
func (n *NodeREPL) Restart() error {
	if err := n.Stop(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return n.Start()
}

// ResizeTerminal resizes the PTY.
func (n *NodeREPL) ResizeTerminal(size process.PTYSize) error {
	if !n.IsRunning() {
		return process.ErrProcessNotRunning
	}

	return n.BaseProcess.ResizePTY(size)
}
