package core

import (
	"fmt"
	"sync"
)

// NodeType represents the type of a node in a topology.
type NodeType int

const (
	// NTSource means the node is a Source.
	NTSource NodeType = iota

	// NTBox means the node is a Box.
	NTBox

	// NTSink means the node is a Sink.
	NTSink
)

func (t NodeType) String() string {
	switch t {
	case NTSource:
		return "source"
	case NTBox:
		return "box"
	case NTSink:
		return "sink"
	default:
		return "unknown"
	}
}

// TopologyState represents a status of a topology or a node.
type TopologyState int

const (
	// TSInitialized means that a topology or a node is just initialized and
	// ready to be run.
	TSInitialized TopologyState = iota

	// TSStarting means a topology or a node is now booting itself and will run
	// shortly.
	TSStarting

	// TSRunning means a topology or a node is currently running and emitting
	// tuples to sinks.
	TSRunning

	// TSPaused means a topology or a node is temporarily stopping to emit
	// tuples and can be resumed later.
	TSPaused

	// TSStopping means a topology or a node is stopping all sources and closing
	// channels between sources, boxes, and sinks.
	TSStopping

	// TSStopped means a topology or a node is stopped. A stopped topology
	// doesn't have to be able to run again.
	TSStopped
)

func (s TopologyState) String() string {
	switch s {
	case TSInitialized:
		return "initialized"
	case TSStarting:
		return "starting"
	case TSRunning:
		return "running"
	case TSPaused:
		return "paused"
	case TSStopping:
		return "stopping"
	case TSStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// TopologyStateHolder is a struct safely referring a state of a topology or a
// node. It only provides read-only methods.
type TopologyStateHolder interface {
	// Get returns the current state of a topology or a node.
	Get() TopologyState

	// Wait waits until the topology or the node has the specified state. It
	// returns the current state. The current state may differ from the given
	// state, but it's guaranteed that the current state is a successor of
	// the given state. For example, when Wait(TSStarting) is called, TSRunning
	// or TSStopped can be returned.
	Wait(s TopologyState) TopologyState
}

type topologyStateHolder struct {
	state TopologyState
	cond  *sync.Cond
}

func newTopologyStateHolder(m sync.Locker) *topologyStateHolder {
	if m == nil {
		m = &sync.Mutex{}
	}
	return &topologyStateHolder{
		cond: sync.NewCond(m),
	}
}

func (h *topologyStateHolder) Get() TopologyState {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	return h.getWithoutLock()
}

func (h *topologyStateHolder) getWithoutLock() TopologyState {
	return h.state
}

// Set sets a new state.
func (h *topologyStateHolder) Set(s TopologyState) error {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	return h.setWithoutLock(s)
}

func (h *topologyStateHolder) setWithoutLock(s TopologyState) error {
	if h.state > s {
		if h.state == TSPaused && s == TSRunning {
			// TSPaused can exceptionally be reset to TSRunning
		} else {
			return fmt.Errorf("state cannot be changed from %v to %v", h.state, s)
		}
	}
	h.state = s
	h.cond.Broadcast()
	return nil
}

func (h *topologyStateHolder) Wait(s TopologyState) TopologyState {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	return h.waitWithoutLock(s)
}

func (h *topologyStateHolder) waitWithoutLock(s TopologyState) TopologyState {
	for {
		if h.state >= s {
			if h.state == TSPaused && s == TSRunning {
				// Wait until the state becomes TSRunning
			} else {
				break
			}
		}
		h.cond.Wait()
	}
	return h.state
}

// checkAndPrepareForRunning checks the current state to see if it can be run.
// It returns the current state and an error. Possible errors are:
//
//	1. a topology or a node is already running (with TSRunning, TSStopped)
//	2. a topology or a node is already stopped (with TSStopped)
//	3. invalid state
//
// The state is set to TSStarting when it can be run.
func (h *topologyStateHolder) checkAndPrepareForRunning() (TopologyState, error) {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	return h.checkAndPrepareForRunningWithoutLock()
}

func (h *topologyStateHolder) checkAndPrepareForRunningWithoutLock() (TopologyState, error) {
	switch h.state {
	case TSInitialized:
		h.setWithoutLock(TSStarting)
		return h.state, nil

	case TSStarting:
		// Immediately returning an error could be confusing for callers,
		// so wait until the topology becomes at least the running state.
		h.waitWithoutLock(TSRunning)

		// It's natural for running methods to return an error when the state
		// isn't TSInitialized even if it's TSStarting so that only a single
		// caller will succeed.
		fallthrough

	case TSRunning, TSPaused:
		return h.state, fmt.Errorf("already running")

	case TSStopping:
		h.waitWithoutLock(TSStopped)
		fallthrough

	case TSStopped:
		return h.state, fmt.Errorf("already stopped")

	default:
		return h.state, fmt.Errorf("invalid state: %v", h.state)
	}
}

// checkAndPrepareForStopping check the current state to see if it can be
// stopped. It returns a bool flag indicating whether a topology or a node
// is already stopped. When it returns false, the caller can stop the component.
// It returns an error only when the current state is invalid.
//
// The state is set to TSStopped when the current state is TSInitialized.
// It might be inconvenient for some components. If the component requires
// termination and cleanup process even if it isn't running, pass true to
// keepInitialized argument. If keep initialized argument is true, it doesn't
// change the state from TSInitialized to TSStopped and returns false and a nil
// error.
func (h *topologyStateHolder) checkAndPrepareForStopping(keepInitialized bool) (alreadyStopped bool, err error) {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	return h.checkAndPrepareForStoppingWithoutLock(keepInitialized)
}

func (h *topologyStateHolder) checkAndPrepareForStoppingWithoutLock(keepInitialized bool) (bool, error) {
	for {
		switch h.state {
		case TSInitialized:
			if keepInitialized {
				return false, nil
			}
			h.setWithoutLock(TSStopped)
			return true, nil

		case TSStarting:
			h.waitWithoutLock(TSRunning)
			// If somebody else has already stopped the component, the state
			// might be different from TSRunning. So, this process continues to
			// the next iteration.

		case TSRunning, TSPaused:
			h.setWithoutLock(TSStopping)
			return false, nil

		case TSStopping:
			// Someone else is trying to stop the component. This thread just
			// waits until it's stopped.
			h.waitWithoutLock(TSStopped)
			return true, nil

		case TSStopped:
			return true, nil

		default:
			return false, fmt.Errorf("invalid state: %v", h.state)
		}
	}
}

// ResumableNode is a node in a topology which can dynamically be paused and
// resumed at runtime.
type ResumableNode interface {
	// Pause pauses a running node. A paused node can be resumed by calling
	// Resume method. Pause is idempotent and pausing a paused node shouldn't
	// fail. Pause may be called before a node runs. For example, when a node
	// is a source, Pause could be called before calling GenerateStream. In
	// that case, GenerateStream should not generate any tuple until Resume is
	// called.
	//
	// When Stop is called while the node is paused, the node must stop without
	// waiting for Resume.
	Pause(ctx *Context) error

	// Resume resumes a paused node. Resume is idempotent and resuming a running
	// node shouldn't fail. Resume may be called before a node runs.
	Resume(ctx *Context) error
}

// BoxInputConfig has parameters to customize input behavior of a Box on each
// input pipe.
type BoxInputConfig struct {
	// InputName is a custom name attached to incoming tuples. When it is empty,
	// "*" will be used.
	InputName string

	// Capacity is the maximum capacity (length) of input pipe. When this
	// parameter is 0, the default value is used. This parameter is only used
	// as a hint and doesn't guarantee that the pipe can actually have the
	// specified number of tuples.
	Capacity int
}

func (c *BoxInputConfig) inputName() string {
	if c.InputName == "" {
		return "*"
	}
	return c.InputName
}

func (c *BoxInputConfig) capacity() int {
	if c.Capacity == 0 {
		return 1024
	}
	return c.Capacity
}

// SinkInputConfig has parameters to customize input behavior of a Sink on
// each input pipe.
type SinkInputConfig struct {
	// Capacity is the maximum capacity (length) of input pipe. When this
	// parameter is 0, the default value is used. This parameter is only used
	// as a hint and doesn't guarantee that the pipe can actually have the
	// specified number of tuples.
	Capacity int
}

func (c *SinkInputConfig) capacity() int {
	if c.Capacity == 0 {
		return 1024
	}
	return c.Capacity
}

var (
	defaultBoxInputConfig  = &BoxInputConfig{}
	defaultSinkInputConfig = &SinkInputConfig{}
)
