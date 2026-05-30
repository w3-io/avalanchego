// Copyright (C) 2026, w3-io, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// EIP-2537 BLS12-381 precompile wiring for the w3 L1.
//
// libevm contains the actual implementations at draft EIP-2537
// addresses (PrecompiledContractsBLS in core/vm/contracts.go).
// The Pectra-final EIP-2537 (March 2025) changed the address
// scheme — there is no separate G1_MUL or G2_MUL precompile, and
// MultiExp was renamed to MSM (Multi-Scalar Multiplication).
//
// This file maps the libevm implementations to Pectra-final
// addresses so Settlement.sol's BLS.sol wrapper finds the
// precompiles where the EIP says they should be.

package params

import (
	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/vm"
)

// Pectra-final EIP-2537 precompile addresses (0x0b–0x11).
var (
	BLS12381G1AddAddress    = common.BytesToAddress([]byte{0x0b})
	BLS12381G1MSMAddress    = common.BytesToAddress([]byte{0x0c})
	BLS12381G2AddAddress    = common.BytesToAddress([]byte{0x0d})
	BLS12381G2MSMAddress    = common.BytesToAddress([]byte{0x0e})
	BLS12381PairingAddress  = common.BytesToAddress([]byte{0x0f})
	BLS12381MapFpToG1Addr   = common.BytesToAddress([]byte{0x10})
	BLS12381MapFp2ToG2Addr  = common.BytesToAddress([]byte{0x11})
)

// libevm's draft EIP-2537 addresses, used here only to look up
// the implementations from libevm's PrecompiledContractsBLS map.
// Pectra-final dropped the standalone G1_MUL and G2_MUL (use MSM
// with one pair instead) and renumbered the rest.
var (
	libevmBLSG1AddDraftAddr      = common.BytesToAddress([]byte{0x0a})
	libevmBLSG1MultiExpDraftAddr = common.BytesToAddress([]byte{0x0c})
	libevmBLSG2AddDraftAddr      = common.BytesToAddress([]byte{0x0d})
	libevmBLSG2MultiExpDraftAddr = common.BytesToAddress([]byte{0x0f})
	libevmBLSPairingDraftAddr    = common.BytesToAddress([]byte{0x10})
	libevmBLSMapG1DraftAddr      = common.BytesToAddress([]byte{0x11})
	libevmBLSMapG2DraftAddr      = common.BytesToAddress([]byte{0x12})
)

// PrecompiledContractsBLS12381 wires libevm's BLS12-381
// implementations into the Pectra-final EIP-2537 address scheme
// expected by Settlement.sol.
//
// Built at init time so we hard-fail at startup if libevm ever
// drops or moves one of these implementations rather than
// silently returning a nil PrecompiledContract on call.
var PrecompiledContractsBLS12381 map[common.Address]vm.PrecompiledContract

func init() {
	mappings := []struct {
		pectraAddr common.Address
		libevmAddr common.Address
		name       string
	}{
		{BLS12381G1AddAddress, libevmBLSG1AddDraftAddr, "BLS12_G1ADD"},
		{BLS12381G1MSMAddress, libevmBLSG1MultiExpDraftAddr, "BLS12_G1MSM"},
		{BLS12381G2AddAddress, libevmBLSG2AddDraftAddr, "BLS12_G2ADD"},
		{BLS12381G2MSMAddress, libevmBLSG2MultiExpDraftAddr, "BLS12_G2MSM"},
		{BLS12381PairingAddress, libevmBLSPairingDraftAddr, "BLS12_PAIRING_CHECK"},
		{BLS12381MapFpToG1Addr, libevmBLSMapG1DraftAddr, "BLS12_MAP_FP_TO_G1"},
		{BLS12381MapFp2ToG2Addr, libevmBLSMapG2DraftAddr, "BLS12_MAP_FP2_TO_G2"},
	}

	PrecompiledContractsBLS12381 = make(map[common.Address]vm.PrecompiledContract, len(mappings))
	for _, m := range mappings {
		impl, ok := vm.PrecompiledContractsBLS[m.libevmAddr]
		if !ok {
			panic("w3 EIP-2537 wiring: libevm.PrecompiledContractsBLS missing implementation for " + m.name + " at draft address " + m.libevmAddr.Hex())
		}
		PrecompiledContractsBLS12381[m.pectraAddr] = impl
	}
}
