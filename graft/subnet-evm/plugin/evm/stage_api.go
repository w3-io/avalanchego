// Copyright (C) 2026, w3.io. w3-io fork only.
// See LICENSE for upstream licensing terms.

// Package evm — "stage" RPC namespace for bootstrap / manufacturing
// tooling. NOT FOR PRODUCTION USE.
//
// When the "stage-api-enabled" config flag is true, this namespace
// exposes a handful of RPC methods that allow an external driver to
// override the VM's wall clock. Without `initial-clock-time`,
// `setClock` directly freezes the underlying mockable clock at the
// requested time. With `initial-clock-time`, the staging clock
// controller owns the clock and advances it at real-wall-time rate;
// `setClock` re-bases the controller and `syncClock` stops it and
// returns to real wall time.
//
// The purpose is pre-network-launch history manufacturing: running
// a local L1 at simulated time to build a coherent past block
// history that matches off-chain manufactured receipts.

package evm

import (
	"fmt"
	"time"
)

// StageAPI is the RPC handler registered under the "stage" namespace
// when config.StageAPIEnabled is true. It deliberately exposes
// minimum surface area — only time manipulation.
type StageAPI struct {
	vm *VM
}

// NewStageAPI constructs the handler.
func NewStageAPI(vm *VM) *StageAPI {
	return &StageAPI{vm: vm}
}

// SetClock overrides the VM's wall clock. Subsequent blocks will be
// stamped at or after the given Unix timestamp (seconds since
// epoch).
//
// When the chain was started with `initial-clock-time`, this
// re-bases the staging clock controller — the clock continues
// advancing at real-wall-time rate from the new base. Otherwise it
// freezes the underlying mockable clock at the given time (note:
// this will stall block production unless real wall time advances
// past it).
//
// Returns the Unix timestamp the VM clock now reports.
func (api *StageAPI) SetClock(unixSeconds uint64) (uint64, error) {
	if api.vm == nil || api.vm.clock == nil {
		return 0, fmt.Errorf("VM clock not available")
	}
	if api.vm.stagingClock != nil {
		return api.vm.stagingClock.Rebase(unixSeconds), nil
	}
	api.vm.clock.Set(time.Unix(int64(unixSeconds), 0))
	return uint64(api.vm.clock.Time().Unix()), nil
}

// SyncClock restores the VM to real wall time.
//
// When a staging clock controller is running, also stops it. After
// SyncClock the underlying clock returns real wall time; only an
// explicit SetClock will re-engage simulated time.
func (api *StageAPI) SyncClock() (uint64, error) {
	if api.vm == nil || api.vm.clock == nil {
		return 0, fmt.Errorf("VM clock not available")
	}
	if api.vm.stagingClock != nil {
		return api.vm.stagingClock.Sync(), nil
	}
	api.vm.clock.Sync()
	return uint64(api.vm.clock.Time().Unix()), nil
}

// Clock returns the VM's current clock value as a Unix timestamp.
// Reflects the simulated value when a staging clock controller is
// running or when SetClock has been called; otherwise real wall
// time.
func (api *StageAPI) Clock() (uint64, error) {
	if api.vm == nil || api.vm.clock == nil {
		return 0, fmt.Errorf("VM clock not available")
	}
	if api.vm.stagingClock != nil {
		return api.vm.stagingClock.CurrentUnix(), nil
	}
	return uint64(api.vm.clock.Time().Unix()), nil
}
