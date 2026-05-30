// Copyright (C) 2026, w3-io, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// EIP-2537 BLS12-381 precompile wiring for the w3 L1.
//
// libevm carries the BLS12-381 implementations under draft EIP-2537
// addresses (0x0a–0x12) with draft gas costs and without input
// subgroup checks. Pectra-final EIP-2537 (March 2025) changed
// three things relative to the draft:
//
//   1. Address scheme: collapsed standalone G1_MUL / G2_MUL into the
//      MSM precompiles (G1_MSM at 0x0c, G2_MSM at 0x0e). Addresses
//      0x0b–0x11 instead of 0x0a–0x12.
//
//   2. Gas costs: lowered most operations, raised the pairing
//      per-pair cost. The mainnet-deployed Pectra values are
//      meaningfully different from libevm's draft constants — for
//      example G2ADD dropped 7.5× (4500 → 600) and MAP_FP2_TO_G2
//      dropped 4.6× (110000 → 23800). Cross-chain bytecode that
//      assumes Pectra gas budgets OOGs on the draft costs.
//
//   3. Input validation: REQUIRES prime-order subgroup checks on
//      every input curve point to every precompile. The draft left
//      this to callers; Pectra mandates it inside the precompile.
//      The check is what makes BLS aggregate signature verification
//      resistant to small-subgroup forgeries (Wagner-class
//      attacks). libevm's implementations do an on-curve check but
//      NOT a subgroup check (except inside the pairing).
//
// This file wraps each libevm implementation in a thin shim that
// (a) returns the Pectra-final gas cost from RequiredGas, and
// (b) subgroup-checks input points before delegating Run to the
// wrapped libevm implementation. The wrapped implementations are
// unchanged — we depend on libevm's audited BLS math.

package params

import (
	"errors"

	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/vm"
	"github.com/ava-labs/libevm/crypto/bls12381"
)

// ---------------------------------------------------------------
// Addresses
// ---------------------------------------------------------------

// Pectra-final EIP-2537 precompile addresses (0x0b–0x11).
var (
	BLS12381G1AddAddress   = common.BytesToAddress([]byte{0x0b})
	BLS12381G1MSMAddress   = common.BytesToAddress([]byte{0x0c})
	BLS12381G2AddAddress   = common.BytesToAddress([]byte{0x0d})
	BLS12381G2MSMAddress   = common.BytesToAddress([]byte{0x0e})
	BLS12381PairingAddress = common.BytesToAddress([]byte{0x0f})
	BLS12381MapFpToG1Addr  = common.BytesToAddress([]byte{0x10})
	BLS12381MapFp2ToG2Addr = common.BytesToAddress([]byte{0x11})
)

// libevm's draft EIP-2537 addresses, used only to look up the
// implementations we wrap. Pectra-final dropped the standalone
// G1_MUL and G2_MUL and renumbered the rest.
var (
	libevmBLSG1AddDraftAddr      = common.BytesToAddress([]byte{0x0a})
	libevmBLSG1MultiExpDraftAddr = common.BytesToAddress([]byte{0x0c})
	libevmBLSG2AddDraftAddr      = common.BytesToAddress([]byte{0x0d})
	libevmBLSG2MultiExpDraftAddr = common.BytesToAddress([]byte{0x0f})
	libevmBLSPairingDraftAddr    = common.BytesToAddress([]byte{0x10})
	libevmBLSMapG1DraftAddr      = common.BytesToAddress([]byte{0x11})
	libevmBLSMapG2DraftAddr      = common.BytesToAddress([]byte{0x12})
)

// ---------------------------------------------------------------
// Pectra-final gas constants
//
// Sources: EIP-2537 final (March 2025) gas section. Mainnet Pectra
// activated these values at block 22,431,084 (April 2025).
// ---------------------------------------------------------------

const (
	pectraG1AddGas       uint64 = 375
	pectraG1MSMBaseGas   uint64 = 12000  // per-pair before discount
	pectraG2AddGas       uint64 = 600
	pectraG2MSMBaseGas   uint64 = 22500  // per-pair before discount
	pectraPairingBaseGas uint64 = 37700  // base cost
	pectraPairingPerPair uint64 = 32600  // per (G1,G2) pair
	pectraMapFpToG1Gas   uint64 = 5500
	pectraMapFp2ToG2Gas  uint64 = 23800
)

// EIP-2537 input element sizes.
const (
	g1PointBytes  = 128 // 2 * 64-byte zero-padded Fp limbs
	g2PointBytes  = 256 // 4 * 64-byte zero-padded Fp limbs
	scalarBytes   = 32  // 256-bit big-endian scalar
	fpBytes       = 64  // single 64-byte zero-padded Fp
	fp2Bytes      = 128 // two 64-byte zero-padded Fp limbs
	g1MSMPairSize = g1PointBytes + scalarBytes // 160
	g2MSMPairSize = g2PointBytes + scalarBytes // 288
	pairingPair   = g1PointBytes + g2PointBytes // 384
)

// ---------------------------------------------------------------
// Errors
// ---------------------------------------------------------------

var (
	errInvalidInputLength = errors.New("invalid input length")
	errEmptyInput         = errors.New("empty input")
	errG1NotInSubgroup    = errors.New("G1 point not in prime-order subgroup")
	errG2NotInSubgroup    = errors.New("G2 point not in prime-order subgroup")
)

// ---------------------------------------------------------------
// Subgroup-check helpers
//
// Allocates a fresh libevm bls12381.G{1,2} per call. The cost is
// trivial relative to the actual BLS math the wrapped precompile
// runs immediately afterwards, and we sidestep concurrency
// concerns (libevm's G{1,2} are not documented as safe for
// concurrent use).
// ---------------------------------------------------------------

func requireG1PointInSubgroup(pointBytes []byte) error {
	g := bls12381.NewG1()
	p, err := g.DecodePoint(pointBytes)
	if err != nil {
		return err
	}
	if !g.InCorrectSubgroup(p) {
		return errG1NotInSubgroup
	}
	return nil
}

func requireG2PointInSubgroup(pointBytes []byte) error {
	g := bls12381.NewG2()
	p, err := g.DecodePoint(pointBytes)
	if err != nil {
		return err
	}
	if !g.InCorrectSubgroup(p) {
		return errG2NotInSubgroup
	}
	return nil
}

// ---------------------------------------------------------------
// Shim type
//
// Each Pectra precompile is a wrapper around a libevm
// PrecompiledContract. RequiredGas returns the Pectra cost; Run
// validates the input shape, subgroup-checks every G1/G2 point in
// the input, then delegates Run to the inner implementation.
//
// Inner.Run never sees out-of-subgroup points because we reject
// them up-front. This is the audit-required behavior — without it
// G1MSM / G2MSM / PAIRING can be tricked into producing or
// validating signatures against points that lie outside the
// prime-order subgroup, breaking BLS's aggregation security.
// ---------------------------------------------------------------

type pectraG1Add struct{ inner vm.PrecompiledContract }

func (c *pectraG1Add) RequiredGas([]byte) uint64 { return pectraG1AddGas }

func (c *pectraG1Add) Run(input []byte) ([]byte, error) {
	if len(input) != 2*g1PointBytes {
		return nil, errInvalidInputLength
	}
	if err := requireG1PointInSubgroup(input[0:g1PointBytes]); err != nil {
		return nil, err
	}
	if err := requireG1PointInSubgroup(input[g1PointBytes : 2*g1PointBytes]); err != nil {
		return nil, err
	}
	return c.inner.Run(input)
}

type pectraG1MSM struct{ inner vm.PrecompiledContract }

func (c *pectraG1MSM) RequiredGas(input []byte) uint64 {
	k := uint64(len(input)) / g1MSMPairSize
	return msmGasCostWithTable(k, pectraG1MSMBaseGas, &bls12381G1MSMDiscountTable)
}

func (c *pectraG1MSM) Run(input []byte) ([]byte, error) {
	if len(input) == 0 || len(input)%g1MSMPairSize != 0 {
		return nil, errInvalidInputLength
	}
	for off := 0; off < len(input); off += g1MSMPairSize {
		if err := requireG1PointInSubgroup(input[off : off+g1PointBytes]); err != nil {
			return nil, err
		}
	}
	return c.inner.Run(input)
}

type pectraG2Add struct{ inner vm.PrecompiledContract }

func (c *pectraG2Add) RequiredGas([]byte) uint64 { return pectraG2AddGas }

func (c *pectraG2Add) Run(input []byte) ([]byte, error) {
	if len(input) != 2*g2PointBytes {
		return nil, errInvalidInputLength
	}
	if err := requireG2PointInSubgroup(input[0:g2PointBytes]); err != nil {
		return nil, err
	}
	if err := requireG2PointInSubgroup(input[g2PointBytes : 2*g2PointBytes]); err != nil {
		return nil, err
	}
	return c.inner.Run(input)
}

type pectraG2MSM struct{ inner vm.PrecompiledContract }

func (c *pectraG2MSM) RequiredGas(input []byte) uint64 {
	k := uint64(len(input)) / g2MSMPairSize
	return msmGasCostWithTable(k, pectraG2MSMBaseGas, &bls12381G2MSMDiscountTable)
}

func (c *pectraG2MSM) Run(input []byte) ([]byte, error) {
	if len(input) == 0 || len(input)%g2MSMPairSize != 0 {
		return nil, errInvalidInputLength
	}
	for off := 0; off < len(input); off += g2MSMPairSize {
		if err := requireG2PointInSubgroup(input[off : off+g2PointBytes]); err != nil {
			return nil, err
		}
	}
	return c.inner.Run(input)
}

type pectraPairing struct{ inner vm.PrecompiledContract }

func (c *pectraPairing) RequiredGas(input []byte) uint64 {
	k := uint64(len(input)) / pairingPair
	return pectraPairingBaseGas + k*pectraPairingPerPair
}

func (c *pectraPairing) Run(input []byte) ([]byte, error) {
	if len(input) == 0 || len(input)%pairingPair != 0 {
		return nil, errInvalidInputLength
	}
	for off := 0; off < len(input); off += pairingPair {
		if err := requireG1PointInSubgroup(input[off : off+g1PointBytes]); err != nil {
			return nil, err
		}
		if err := requireG2PointInSubgroup(input[off+g1PointBytes : off+pairingPair]); err != nil {
			return nil, err
		}
	}
	return c.inner.Run(input)
}

// pectraMapFpToG1 and pectraMapFp2ToG2 take field elements, not
// curve points, so there is no input subgroup check to perform.
// SSWU + clear_cofactor guarantees the output is in the prime-
// order subgroup; libevm's implementations handle this internally.
type pectraMapFpToG1 struct{ inner vm.PrecompiledContract }

func (c *pectraMapFpToG1) RequiredGas([]byte) uint64 { return pectraMapFpToG1Gas }

func (c *pectraMapFpToG1) Run(input []byte) ([]byte, error) {
	if len(input) != fpBytes {
		return nil, errInvalidInputLength
	}
	return c.inner.Run(input)
}

type pectraMapFp2ToG2 struct{ inner vm.PrecompiledContract }

func (c *pectraMapFp2ToG2) RequiredGas([]byte) uint64 { return pectraMapFp2ToG2Gas }

func (c *pectraMapFp2ToG2) Run(input []byte) ([]byte, error) {
	if len(input) != fp2Bytes {
		return nil, errInvalidInputLength
	}
	return c.inner.Run(input)
}

// ---------------------------------------------------------------
// MSM gas formula
//
// Pectra-final: cost = k * base * discount(k) / 1000
//
// Pectra-final spec ships TWO discount tables — one for G1 and
// one for G2 — and the libevm table (draft EIP-2537) starting
// at 1200 is WRONG to use here. libevm's table predates the
// Pectra-final spec and bears no relation to the deployed
// mainnet costs.
//
// These tables match the canonical go-ethereum
// params/protocol_params.go Bls12381G{1,2}MultiExpDiscountTable
// constants used on mainnet from Pectra onward (~April 2025).
//
// discount(k) for k > 128 is the table's last entry (the
// "max_discount" cap from the EIP).
// ---------------------------------------------------------------

var bls12381G1MSMDiscountTable = [128]uint64{
	1000, 949, 848, 797, 764, 750, 738, 728, 719, 712,
	705, 698, 692, 687, 682, 677, 673, 669, 665, 661,
	658, 654, 651, 648, 645, 642, 640, 637, 635, 632,
	630, 627, 625, 623, 621, 619, 617, 615, 613, 611,
	609, 608, 606, 604, 603, 601, 599, 598, 596, 595,
	593, 592, 591, 589, 588, 586, 585, 584, 582, 581,
	580, 579, 577, 576, 575, 574, 573, 572, 570, 569,
	568, 567, 566, 565, 564, 563, 562, 561, 560, 559,
	558, 557, 556, 555, 554, 553, 552, 551, 550, 549,
	548, 547, 547, 546, 545, 544, 543, 542, 541, 540,
	540, 539, 538, 537, 536, 536, 535, 534, 533, 532,
	532, 531, 530, 529, 528, 528, 527, 526, 525, 525,
	524, 523, 522, 522, 521, 520, 520, 519,
}

var bls12381G2MSMDiscountTable = [128]uint64{
	1000, 1000, 923, 884, 855, 832, 812, 796, 782, 770,
	759, 749, 740, 732, 724, 717, 711, 704, 699, 693,
	688, 683, 679, 674, 670, 666, 663, 659, 655, 652,
	649, 646, 643, 640, 637, 634, 632, 629, 627, 624,
	622, 620, 618, 615, 613, 611, 609, 607, 606, 604,
	602, 600, 598, 597, 595, 593, 592, 590, 589, 587,
	586, 584, 583, 582, 580, 579, 578, 576, 575, 574,
	573, 571, 570, 569, 568, 567, 566, 565, 563, 562,
	561, 560, 559, 558, 557, 556, 555, 554, 553, 552,
	552, 551, 550, 549, 548, 547, 546, 545, 545, 544,
	543, 542, 541, 541, 540, 539, 538, 537, 537, 536,
	535, 535, 534, 533, 532, 532, 531, 530, 530, 529,
	528, 528, 527, 526, 526, 525, 524, 524,
}

func msmGasCostWithTable(k, perPair uint64, table *[128]uint64) uint64 {
	if k == 0 {
		return 0
	}
	idx := k - 1
	if idx >= uint64(len(table)) {
		idx = uint64(len(table) - 1)
	}
	discount := table[idx]
	return (k * perPair * discount) / 1000
}

// ---------------------------------------------------------------
// Map (built at init)
// ---------------------------------------------------------------

// precompiledContractsBLS12381 wires each Pectra-final EIP-2537
// address to a shim that enforces Pectra gas + subgroup checks
// over libevm's BLS implementation.
//
// Built at init time so we hard-fail at startup if libevm ever
// drops or moves one of the underlying implementations.
var precompiledContractsBLS12381 map[common.Address]vm.PrecompiledContract

func init() {
	lookup := func(libevmAddr common.Address, name string) vm.PrecompiledContract {
		impl, ok := vm.PrecompiledContractsBLS[libevmAddr]
		if !ok {
			panic("w3 EIP-2537 wiring: libevm.PrecompiledContractsBLS missing implementation for " + name + " at draft address " + libevmAddr.Hex())
		}
		return impl
	}

	precompiledContractsBLS12381 = map[common.Address]vm.PrecompiledContract{
		BLS12381G1AddAddress:   &pectraG1Add{inner: lookup(libevmBLSG1AddDraftAddr, "BLS12_G1ADD")},
		BLS12381G1MSMAddress:   &pectraG1MSM{inner: lookup(libevmBLSG1MultiExpDraftAddr, "BLS12_G1MSM")},
		BLS12381G2AddAddress:   &pectraG2Add{inner: lookup(libevmBLSG2AddDraftAddr, "BLS12_G2ADD")},
		BLS12381G2MSMAddress:   &pectraG2MSM{inner: lookup(libevmBLSG2MultiExpDraftAddr, "BLS12_G2MSM")},
		BLS12381PairingAddress: &pectraPairing{inner: lookup(libevmBLSPairingDraftAddr, "BLS12_PAIRING_CHECK")},
		BLS12381MapFpToG1Addr:  &pectraMapFpToG1{inner: lookup(libevmBLSMapG1DraftAddr, "BLS12_MAP_FP_TO_G1")},
		BLS12381MapFp2ToG2Addr: &pectraMapFp2ToG2{inner: lookup(libevmBLSMapG2DraftAddr, "BLS12_MAP_FP2_TO_G2")},
	}
}
