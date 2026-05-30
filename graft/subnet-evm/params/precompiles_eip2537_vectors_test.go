// Copyright (C) 2026, w3-io, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// EIP-2537 reference-vector tests for the Pectra shim.
//
// We vendor a subset of libevm's EIP-2537 test vectors (which are
// themselves derived from the go-ethereum / ethereum-tests
// authoritative test suite) under testdata/eip2537/. The positive
// vectors (blsG1Add.json etc.) assert byte-for-byte parity between
// our shim's Run output and the spec'd expected output. The
// failure vectors (fail-bls*.json) assert that malformed inputs
// (wrong length, off-curve, top-byte violations) are rejected.
//
// In addition, the dedicated subgroup-rejection tests below
// extract the known non-prime-order G1 and G2 points from the
// pairing failure vectors and feed them into each of our
// shim's group precompiles (G1ADD/G1MSM/G2ADD/G2MSM/PAIRING) to
// prove the subgroup check we added in v1.14.2-w3.2 actually
// fires on real out-of-subgroup inputs.

package params

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/ava-labs/libevm/common"
)

// ---------------------------------------------------------------
// Vector loading
// ---------------------------------------------------------------

//go:embed testdata/eip2537/blsG1Add.json
var posG1AddJSON []byte

//go:embed testdata/eip2537/blsG1MultiExp.json
var posG1MSMJSON []byte

//go:embed testdata/eip2537/blsG2Add.json
var posG2AddJSON []byte

//go:embed testdata/eip2537/blsG2MultiExp.json
var posG2MSMJSON []byte

//go:embed testdata/eip2537/blsPairing.json
var posPairingJSON []byte

//go:embed testdata/eip2537/blsMapG1.json
var posMapG1JSON []byte

//go:embed testdata/eip2537/blsMapG2.json
var posMapG2JSON []byte

//go:embed testdata/eip2537/fail-blsG1Add.json
var negG1AddJSON []byte

//go:embed testdata/eip2537/fail-blsG1MultiExp.json
var negG1MSMJSON []byte

//go:embed testdata/eip2537/fail-blsG2Add.json
var negG2AddJSON []byte

//go:embed testdata/eip2537/fail-blsG2MultiExp.json
var negG2MSMJSON []byte

//go:embed testdata/eip2537/fail-blsPairing.json
var negPairingJSON []byte

//go:embed testdata/eip2537/fail-blsMapG1.json
var negMapG1JSON []byte

//go:embed testdata/eip2537/fail-blsMapG2.json
var negMapG2JSON []byte

type positiveVector struct {
	Input    string `json:"Input"`
	Expected string `json:"Expected"`
	Name     string `json:"Name"`
}

type negativeVector struct {
	Input         string `json:"Input"`
	ExpectedError string `json:"ExpectedError"`
	Name          string `json:"Name"`
}

func loadPositive(t *testing.T, raw []byte) []positiveVector {
	t.Helper()
	var v []positiveVector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("load positive vectors: %v", err)
	}
	return v
}

func loadNegative(t *testing.T, raw []byte) []negativeVector {
	t.Helper()
	var v []negativeVector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("load negative vectors: %v", err)
	}
	return v
}

// ---------------------------------------------------------------
// Positive parity tests
//
// For each vector: hex-decode Input, call our shim's Run, and
// assert the output matches Expected byte-for-byte. This proves
// the shim doesn't alter libevm's underlying BLS math while
// adding gas + subgroup-check enforcement.
//
// Gas costs from these vectors are libevm-draft and intentionally
// ignored here; the Pectra-final gas table is enforced in
// TestPrecompiledContractsBLS12381_PectraGasCosts.
// ---------------------------------------------------------------

func runPositiveSuite(t *testing.T, addr common.Address, raw []byte) {
	impl := precompiledContractsBLS12381[addr]
	vectors := loadPositive(t, raw)
	for _, v := range vectors {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			input, err := hex.DecodeString(v.Input)
			if err != nil {
				t.Fatalf("decode input: %v", err)
			}
			want, err := hex.DecodeString(v.Expected)
			if err != nil {
				t.Fatalf("decode expected: %v", err)
			}
			got, err := impl.Run(input)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output mismatch\n got=%x\nwant=%x", got, want)
			}
		})
	}
}

func TestVectors_Positive_G1Add(t *testing.T) {
	runPositiveSuite(t, BLS12381G1AddAddress, posG1AddJSON)
}

func TestVectors_Positive_G1MSM(t *testing.T) {
	runPositiveSuite(t, BLS12381G1MSMAddress, posG1MSMJSON)
}

func TestVectors_Positive_G2Add(t *testing.T) {
	runPositiveSuite(t, BLS12381G2AddAddress, posG2AddJSON)
}

func TestVectors_Positive_G2MSM(t *testing.T) {
	runPositiveSuite(t, BLS12381G2MSMAddress, posG2MSMJSON)
}

func TestVectors_Positive_Pairing(t *testing.T) {
	runPositiveSuite(t, BLS12381PairingAddress, posPairingJSON)
}

func TestVectors_Positive_MapFpToG1(t *testing.T) {
	runPositiveSuite(t, BLS12381MapFpToG1Addr, posMapG1JSON)
}

func TestVectors_Positive_MapFp2ToG2(t *testing.T) {
	runPositiveSuite(t, BLS12381MapFp2ToG2Addr, posMapG2JSON)
}

// ---------------------------------------------------------------
// Negative tests
//
// Each fail-* vector specifies an input the spec says MUST be
// rejected. Our shim catches some of these earlier than libevm
// (e.g., short input is rejected in the shim before delegating)
// and delegates others to libevm. Either way, Run MUST return a
// non-nil error.
//
// We don't compare error messages — the shim's "invalid input
// length" wording differs from libevm's "invalid number of bytes"
// for length errors, and that's fine: the audit-relevant property
// is rejection, not message text.
// ---------------------------------------------------------------

func runNegativeSuite(t *testing.T, addr common.Address, raw []byte) {
	impl := precompiledContractsBLS12381[addr]
	vectors := loadNegative(t, raw)
	for _, v := range vectors {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			input, err := hex.DecodeString(v.Input)
			if err != nil {
				t.Fatalf("decode input: %v", err)
			}
			if _, err := impl.Run(input); err == nil {
				t.Errorf("expected error for vector %q (libevm: %q), got nil",
					v.Name, v.ExpectedError)
			}
		})
	}
}

func TestVectors_Negative_G1Add(t *testing.T) {
	runNegativeSuite(t, BLS12381G1AddAddress, negG1AddJSON)
}

func TestVectors_Negative_G1MSM(t *testing.T) {
	runNegativeSuite(t, BLS12381G1MSMAddress, negG1MSMJSON)
}

func TestVectors_Negative_G2Add(t *testing.T) {
	runNegativeSuite(t, BLS12381G2AddAddress, negG2AddJSON)
}

func TestVectors_Negative_G2MSM(t *testing.T) {
	runNegativeSuite(t, BLS12381G2MSMAddress, negG2MSMJSON)
}

func TestVectors_Negative_Pairing(t *testing.T) {
	runNegativeSuite(t, BLS12381PairingAddress, negPairingJSON)
}

func TestVectors_Negative_MapFpToG1(t *testing.T) {
	runNegativeSuite(t, BLS12381MapFpToG1Addr, negMapG1JSON)
}

func TestVectors_Negative_MapFp2ToG2(t *testing.T) {
	runNegativeSuite(t, BLS12381MapFp2ToG2Addr, negMapG2JSON)
}

// ---------------------------------------------------------------
// Subgroup rejection (the headline C2 security check)
//
// The non-subgroup G1 and G2 points are extracted from the
// authoritative pairing failure vectors named
// "bls_pairing_g{1,2}_not_in_correct_subgroup". These vectors
// contain a valid (subgroup) point in pair 0 and the
// out-of-subgroup point in pair 1. We pull the offending point
// out and feed it into our G1ADD / G1MSM / G2ADD / G2MSM shims
// to prove they reject it — libevm's underlying implementations
// don't subgroup-check inputs to these precompiles, so without
// our shim's wrapper the call would succeed and feed a
// non-prime-order point into downstream BLS pipelines.
// ---------------------------------------------------------------

func findVector(t *testing.T, raw []byte, name string) negativeVector {
	t.Helper()
	vectors := loadNegative(t, raw)
	for _, v := range vectors {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("vector %q not found", name)
	return negativeVector{}
}

// nonSubgroupG1 returns the 128-byte encoded G1 point that the
// pairing failure vector identifies as not-in-correct-subgroup.
// It's the G1 element of the second pair (offset 384 bytes / 768
// hex chars; G1 portion is 128 bytes / 256 hex chars).
func nonSubgroupG1(t *testing.T) []byte {
	t.Helper()
	v := findVector(t, negPairingJSON, "bls_pairing_g1_not_in_correct_subgroup")
	const g1Start = 768
	const g1End = g1Start + 256
	if len(v.Input) < g1End {
		t.Fatalf("pairing vector input too short: %d hex chars", len(v.Input))
	}
	b, err := hex.DecodeString(v.Input[g1Start:g1End])
	if err != nil {
		t.Fatalf("decode non-subgroup G1: %v", err)
	}
	if len(b) != g1PointBytes {
		t.Fatalf("non-subgroup G1 wrong length: got %d, want %d", len(b), g1PointBytes)
	}
	return b
}

// nonSubgroupG2 returns the 256-byte encoded G2 point that the
// pairing failure vector identifies as not-in-correct-subgroup.
// G2 portion of the second pair: hex offset 768 + 256 = 1024 ..
// 1024 + 512 = 1536.
func nonSubgroupG2(t *testing.T) []byte {
	t.Helper()
	v := findVector(t, negPairingJSON, "bls_pairing_g2_not_in_correct_subgroup")
	const g2Start = 1024
	const g2End = g2Start + 512
	if len(v.Input) < g2End {
		t.Fatalf("pairing vector input too short: %d hex chars", len(v.Input))
	}
	b, err := hex.DecodeString(v.Input[g2Start:g2End])
	if err != nil {
		t.Fatalf("decode non-subgroup G2: %v", err)
	}
	if len(b) != g2PointBytes {
		t.Fatalf("non-subgroup G2 wrong length: got %d, want %d", len(b), g2PointBytes)
	}
	return b
}

func TestSubgroupRejection_G1ADD(t *testing.T) {
	bad := nonSubgroupG1(t)
	identity := make([]byte, g1PointBytes)
	input := append(append([]byte{}, bad...), identity...)
	impl := precompiledContractsBLS12381[BLS12381G1AddAddress]
	_, err := impl.Run(input)
	if err == nil {
		t.Fatal("expected G1ADD to reject non-subgroup G1 input, got nil error")
	}
	if err != errG1NotInSubgroup {
		t.Errorf("expected errG1NotInSubgroup, got %v", err)
	}
}

func TestSubgroupRejection_G1MSM(t *testing.T) {
	bad := nonSubgroupG1(t)
	scalar := scalar1()
	input := append(append([]byte{}, bad...), scalar...)
	impl := precompiledContractsBLS12381[BLS12381G1MSMAddress]
	_, err := impl.Run(input)
	if err == nil {
		t.Fatal("expected G1MSM to reject non-subgroup G1 input, got nil error")
	}
	if err != errG1NotInSubgroup {
		t.Errorf("expected errG1NotInSubgroup, got %v", err)
	}
}

func TestSubgroupRejection_G2ADD(t *testing.T) {
	bad := nonSubgroupG2(t)
	identity := make([]byte, g2PointBytes)
	input := append(append([]byte{}, bad...), identity...)
	impl := precompiledContractsBLS12381[BLS12381G2AddAddress]
	_, err := impl.Run(input)
	if err == nil {
		t.Fatal("expected G2ADD to reject non-subgroup G2 input, got nil error")
	}
	if err != errG2NotInSubgroup {
		t.Errorf("expected errG2NotInSubgroup, got %v", err)
	}
}

func TestSubgroupRejection_G2MSM(t *testing.T) {
	bad := nonSubgroupG2(t)
	scalar := scalar1()
	input := append(append([]byte{}, bad...), scalar...)
	impl := precompiledContractsBLS12381[BLS12381G2MSMAddress]
	_, err := impl.Run(input)
	if err == nil {
		t.Fatal("expected G2MSM to reject non-subgroup G2 input, got nil error")
	}
	if err != errG2NotInSubgroup {
		t.Errorf("expected errG2NotInSubgroup, got %v", err)
	}
}

func TestSubgroupRejection_Pairing_G1(t *testing.T) {
	bad := nonSubgroupG1(t)
	g2 := g2Generator(t)
	input := append(append([]byte{}, bad...), g2...)
	impl := precompiledContractsBLS12381[BLS12381PairingAddress]
	_, err := impl.Run(input)
	if err == nil {
		t.Fatal("expected PAIRING to reject non-subgroup G1 input, got nil error")
	}
	if err != errG1NotInSubgroup {
		t.Errorf("expected errG1NotInSubgroup, got %v", err)
	}
}

func TestSubgroupRejection_Pairing_G2(t *testing.T) {
	g1 := g1Generator(t)
	bad := nonSubgroupG2(t)
	input := append(append([]byte{}, g1...), bad...)
	impl := precompiledContractsBLS12381[BLS12381PairingAddress]
	_, err := impl.Run(input)
	if err == nil {
		t.Fatal("expected PAIRING to reject non-subgroup G2 input, got nil error")
	}
	if err != errG2NotInSubgroup {
		t.Errorf("expected errG2NotInSubgroup, got %v", err)
	}
}
