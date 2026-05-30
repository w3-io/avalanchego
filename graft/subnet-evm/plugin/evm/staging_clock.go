// Copyright (C) 2026, w3.io. w3-io fork only.
// See LICENSE for upstream licensing terms.

// Staging clock controller for the w3-io subnet-evm fork.
//
// `mockable.Clock.Set(t)` freezes the clock at `t` until another
// call. Block production needs a clock that *advances* — otherwise
// bootstrap blocks can't be mined. This file wires an advancing
// fake clock: when `initial-clock-time` is set, the controller
// pins the simulated time at the given start, then ticks it
// forward at real-wall-time rate.
//
// `stage_setClock` re-bases the clock and the controller continues
// from the new base. `stage_syncClock` stops the controller and
// returns the underlying clock to real wall time.

package evm

import (
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/utils/timer/mockable"
	"github.com/ava-labs/libevm/log"
)

// stagingClockTickInterval — how often the controller refreshes
// vm.clock from its base. Smaller = finer-grained simulated time;
// 100ms is plenty for our 1-2s block intervals.
const stagingClockTickInterval = 100 * time.Millisecond

// stagingClockController owns an advancing fake clock. Multiple
// callers (init, stage_setClock RPC, stage_syncClock RPC) interact
// via Rebase / Sync.
type stagingClockController struct {
	mu             sync.Mutex
	clock          *mockable.Clock
	baseSimulated  time.Time
	baseReal       time.Time
	stopCh         chan struct{}
	running        bool
}

// newStagingClockController initializes the controller and starts
// the advancement goroutine. The clock is immediately set to
// `initialUnixSeconds`; subsequent ticks add real-wall elapsed
// time to that base.
func newStagingClockController(c *mockable.Clock, initialUnixSeconds uint64) *stagingClockController {
	ctrl := &stagingClockController{
		clock:         c,
		baseSimulated: time.Unix(int64(initialUnixSeconds), 0),
		baseReal:      time.Now(),
		stopCh:        make(chan struct{}),
		running:       true,
	}
	c.Set(ctrl.baseSimulated)
	go ctrl.run()
	log.Warn(
		"staging clock controller started — chain advances in simulated time",
		"initial_unix_seconds", initialUnixSeconds,
	)
	return ctrl
}

func (c *stagingClockController) run() {
	ticker := time.NewTicker(stagingClockTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.mu.Lock()
			if !c.running {
				c.mu.Unlock()
				continue
			}
			elapsed := time.Since(c.baseReal)
			c.clock.Set(c.baseSimulated.Add(elapsed))
			c.mu.Unlock()
		}
	}
}

// Rebase resets the simulated time to the given unix seconds.
// Subsequent ticks advance from the new base. Returns the clock's
// new value (in unix seconds), which equals `unixSeconds` modulo
// the controller's tick granularity.
func (c *stagingClockController) Rebase(unixSeconds uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseSimulated = time.Unix(int64(unixSeconds), 0)
	c.baseReal = time.Now()
	c.running = true
	c.clock.Set(c.baseSimulated)
	return uint64(c.clock.Time().Unix())
}

// Sync stops the controller and returns the clock to real wall
// time. After Sync, only an explicit Rebase will resume simulated
// time advancement.
func (c *stagingClockController) Sync() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.clock.Sync()
	return uint64(c.clock.Time().Unix())
}

// CurrentUnix returns the controller's current simulated unix seconds.
func (c *stagingClockController) CurrentUnix() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return uint64(c.clock.Time().Unix())
}

// startStagingClock is called from VM Initialize. It attaches the
// controller to the VM so the stage RPC handlers can reach it.
func startStagingClock(vm *VM, initialUnixSeconds uint64) {
	vm.stagingClock = newStagingClockController(vm.clock, initialUnixSeconds)
}
