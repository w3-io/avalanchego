// Copyright (C) 2026, w3-io, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ava-labs/libevm/common"
	"github.com/ava-labs/libevm/core/vm"
)

// G1 generator (x || y, each 64-byte zero-padded Fp). Constants
// from EIP-2537 §"BLS12-381 generators".
const g1GeneratorHex = "" +
	"0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb" +
	"0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e1"

// G2 generator (x_c0 || x_c1 || y_c0 || y_c1, each 64 bytes).
// Each Fp element is 48 bytes (96 hex chars) zero-padded to 64.
const g2GeneratorHex = "" +
	"00000000000000000000000000000000024aa2b2f08f0a91260805272dc51051c6e47ad4fa403b02b4510b647ae3d1770bac0326a805bbefd48056c8c121bdb8" +
	"0000000000000000000000000000000013e02b6052719f607dacd3a088274f65596bd0d09920b61ab5da61bbdc7f5049334cf11213945d57e5ac7d055d042b7e" +
	"000000000000000000000000000000000ce5d527727d6e118cc9cdc6da2e351aadfd9baa8cbdd3a76d429a695160d12c923ac9cc3baca289e193548608b82801" +
	"000000000000000000000000000000000606c4a02ea734cc32acd2b02bc28b99cb3e287e85a763af267492ab572e99ab3f370d275cec1da1aaa9075ff05f79be"

func g1Generator(t *testing.T) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(g1GeneratorHex, " ", ""))
	if err != nil {
		t.Fatalf("decode G1 generator: %v", err)
	}
	return b
}

func g2Generator(t *testing.T) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(g2GeneratorHex, " ", ""))
	if err != nil {
		t.Fatalf("decode G2 generator: %v", err)
	}
	return b
}

func scalar1() []byte {
	s := make([]byte, scalarBytes)
	s[scalarBytes-1] = 1
	return s
}

// ---------------------------------------------------------------
// Address scheme
// ---------------------------------------------------------------

func TestPrecompiledContractsBLS12381_AddressScheme(t *testing.T) {
	expected := map[common.Address]string{
		BLS12381G1AddAddress:   "G1ADD",
		BLS12381G1MSMAddress:   "G1MSM",
		BLS12381G2AddAddress:   "G2ADD",
		BLS12381G2MSMAddress:   "G2MSM",
		BLS12381PairingAddress: "PAIRING_CHECK",
		BLS12381MapFpToG1Addr:  "MAP_FP_TO_G1",
		BLS12381MapFp2ToG2Addr: "MAP_FP2_TO_G2",
	}

	if got, want := len(precompiledContractsBLS12381), len(expected); got != want {
		t.Fatalf("precompiledContractsBLS12381 size = %d, want %d", got, want)
	}
	for addr, name := range expected {
		if _, ok := precompiledContractsBLS12381[addr]; !ok {
			t.Errorf("missing %s at address %s", name, addr.Hex())
		}
	}
	for addr := range precompiledContractsBLS12381 {
		if _, ok := expected[addr]; !ok {
			t.Errorf("unexpected precompile at address %s", addr.Hex())
		}
	}
	for addr := range precompiledContractsBLS12381 {
		lastByte := addr.Bytes()[len(addr.Bytes())-1]
		if lastByte < 0x0b || lastByte > 0x11 {
			t.Errorf("BLS precompile at %s outside Pectra-final EIP-2537 range 0x0b-0x11", addr.Hex())
		}
	}
}

// ---------------------------------------------------------------
// Pectra-final gas costs
//
// Cross-chain bytecode tuned for Pectra-final mainnet costs will
// hit out-of-gas if we charge libevm's draft costs. These tests
// pin the contract to the spec'd values.
// ---------------------------------------------------------------

func TestPrecompiledContractsBLS12381_PectraGasCosts(t *testing.T) {
	g1 := g1Generator(t)
	g2 := g2Generator(t)

	// Fixed-cost precompiles.
	fixedCases := []struct {
		name       string
		addr       common.Address
		input      []byte
		wantGas    uint64
		libevmGas  uint64 // libevm draft, for clarity
	}{
		{"G1ADD", BLS12381G1AddAddress, append(append([]byte{}, g1...), g1...), 375, 600},
		{"G2ADD", BLS12381G2AddAddress, append(append([]byte{}, g2...), g2...), 600, 4500},
		{"MAP_FP_TO_G1", BLS12381MapFpToG1Addr, make([]byte, fpBytes), 5500, 5500},
		{"MAP_FP2_TO_G2", BLS12381MapFp2ToG2Addr, make([]byte, fp2Bytes), 23800, 110000},
	}
	for _, c := range fixedCases {
		t.Run(c.name+"/gas", func(t *testing.T) {
			impl := precompiledContractsBLS12381[c.addr]
			got := impl.RequiredGas(c.input)
			if got != c.wantGas {
				t.Errorf("%s.RequiredGas = %d, want Pectra-final %d (libevm draft would be %d)",
					c.name, got, c.wantGas, c.libevmGas)
			}
		})
	}

	// G1MSM at k=1, 2, 5. Formula: k * 12000 * discount(k) / 1000.
	// Discount table indices are 0-based on k, so discount(k) is
	// table[k-1]: discount(1)=1200, discount(2)=888, discount(5)=594.
	g1MSM := precompiledContractsBLS12381[BLS12381G1MSMAddress]
	g1MSMCases := []struct {
		k       uint64
		wantGas uint64
	}{
		{1, (1 * 12000 * 1200) / 1000},
		{2, (2 * 12000 * 888) / 1000},
		{5, (5 * 12000 * 594) / 1000},
	}
	for _, c := range g1MSMCases {
		t.Run("G1MSM/gas/k="+itoa(c.k), func(t *testing.T) {
			input := make([]byte, c.k*g1MSMPairSize)
			if got := g1MSM.RequiredGas(input); got != c.wantGas {
				t.Errorf("G1MSM(k=%d).RequiredGas = %d, want %d", c.k, got, c.wantGas)
			}
		})
	}

	// G2MSM at k=1, 2, 5. Formula: k * 22500 * discount(k) / 1000.
	// Pectra base is 22500; libevm draft G2MUL base is 55000 — must
	// override.
	g2MSM := precompiledContractsBLS12381[BLS12381G2MSMAddress]
	g2MSMCases := []struct {
		k       uint64
		wantGas uint64
	}{
		{1, (1 * 22500 * 1200) / 1000},
		{2, (2 * 22500 * 888) / 1000},
		{5, (5 * 22500 * 594) / 1000},
	}
	for _, c := range g2MSMCases {
		t.Run("G2MSM/gas/k="+itoa(c.k), func(t *testing.T) {
			input := make([]byte, c.k*g2MSMPairSize)
			if got := g2MSM.RequiredGas(input); got != c.wantGas {
				t.Errorf("G2MSM(k=%d).RequiredGas = %d, want %d", c.k, got, c.wantGas)
			}
		})
	}

	// PAIRING_CHECK at k=1, 2, 4. Formula: 37700 + 32600*k.
	// Pectra: base 37700, per-pair 32600. libevm: 115000 + 23000*k.
	pairing := precompiledContractsBLS12381[BLS12381PairingAddress]
	pairingCases := []struct {
		k       uint64
		wantGas uint64
	}{
		{1, 37700 + 32600*1},
		{2, 37700 + 32600*2},
		{4, 37700 + 32600*4},
	}
	for _, c := range pairingCases {
		t.Run("PAIRING/gas/k="+itoa(c.k), func(t *testing.T) {
			input := make([]byte, c.k*pairingPair)
			if got := pairing.RequiredGas(input); got != c.wantGas {
				t.Errorf("PAIRING(k=%d).RequiredGas = %d, want %d (libevm draft would be %d)",
					c.k, got, c.wantGas, 115000+23000*c.k)
			}
		})
	}
}

// ---------------------------------------------------------------
// Routing parity
//
// Valid subgroup-checked inputs should produce the same bytes from
// our shim as from libevm's underlying implementation. This is the
// regression net for "we don't accidentally alter libevm's BLS
// math when wrapping it."
// ---------------------------------------------------------------

func TestPrecompiledContractsBLS12381_ParityWithLibevm(t *testing.T) {
	g1 := g1Generator(t)
	g2 := g2Generator(t)

	cases := []struct {
		name       string
		pectraAddr common.Address
		libevmAddr common.Address
		input      []byte
	}{
		{"G1ADD", BLS12381G1AddAddress, libevmBLSG1AddDraftAddr,
			append(append([]byte{}, g1...), g1...)},
		{"G2ADD", BLS12381G2AddAddress, libevmBLSG2AddDraftAddr,
			append(append([]byte{}, g2...), g2...)},
		{"G1MSM/k=1", BLS12381G1MSMAddress, libevmBLSG1MultiExpDraftAddr,
			append(append([]byte{}, g1...), scalar1()...)},
		{"G2MSM/k=1", BLS12381G2MSMAddress, libevmBLSG2MultiExpDraftAddr,
			append(append([]byte{}, g2...), scalar1()...)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pectraImpl, ok := precompiledContractsBLS12381[c.pectraAddr]
			if !ok {
				t.Fatalf("no precompile wired at %s", c.pectraAddr.Hex())
			}
			libevmImpl := vm.PrecompiledContractsBLS[c.libevmAddr]

			gotPectra, errPectra := pectraImpl.Run(c.input)
			gotLibevm, errLibevm := libevmImpl.Run(c.input)
			if errPectra != nil || errLibevm != nil {
				t.Fatalf("Run error: pectra=%v libevm=%v", errPectra, errLibevm)
			}
			if !bytes.Equal(gotPectra, gotLibevm) {
				t.Errorf("output mismatch: pectra=%x libevm=%x", gotPectra, gotLibevm)
			}
		})
	}
}

// ---------------------------------------------------------------
// Input validation
//
// Each shim rejects malformed-length input BEFORE delegating to
// libevm. Pectra gas is charged via RequiredGas regardless of
// validity (caller pays for the attempted call), so we test
// behavioral rejection by Run.
// ---------------------------------------------------------------

func TestPrecompiledContractsBLS12381_RejectsBadInputLength(t *testing.T) {
	cases := []struct {
		name string
		addr common.Address
		bad  []byte
	}{
		{"G1ADD/empty", BLS12381G1AddAddress, nil},
		{"G1ADD/short", BLS12381G1AddAddress, make([]byte, g1PointBytes-1)},
		{"G2ADD/empty", BLS12381G2AddAddress, nil},
		{"G1MSM/empty", BLS12381G1MSMAddress, nil},
		{"G1MSM/nonMultiple", BLS12381G1MSMAddress, make([]byte, g1MSMPairSize+1)},
		{"G2MSM/empty", BLS12381G2MSMAddress, nil},
		{"PAIRING/empty", BLS12381PairingAddress, nil},
		{"PAIRING/nonMultiple", BLS12381PairingAddress, make([]byte, pairingPair-1)},
		{"MAP_FP_TO_G1/short", BLS12381MapFpToG1Addr, make([]byte, fpBytes-1)},
		{"MAP_FP2_TO_G2/short", BLS12381MapFp2ToG2Addr, make([]byte, fp2Bytes-1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			impl := precompiledContractsBLS12381[c.addr]
			if _, err := impl.Run(c.bad); err == nil {
				t.Errorf("%s: expected error on bad input length, got nil", c.name)
			}
		})
	}
}

// ---------------------------------------------------------------
// Subgroup check invocation
//
// The G1 / G2 zero buffer (identity element in EIP-2537 encoding)
// is a subgroup-valid point: it's a member of every subgroup
// trivially. Subgroup-checking it succeeds, and libevm's Run on a
// double-identity input returns the identity. We use this to
// assert the subgroup check doesn't reject valid inputs; the
// negative-rejection case (on-curve but non-prime-order) is
// covered by the EIP-2537 reference vectors added in W3-727.
// ---------------------------------------------------------------

func TestPrecompiledContractsBLS12381_AcceptsIdentityG1(t *testing.T) {
	impl := precompiledContractsBLS12381[BLS12381G1AddAddress]
	zero := make([]byte, g1PointBytes)
	input := append(append([]byte{}, zero...), zero...)
	out, err := impl.Run(input)
	if err != nil {
		t.Fatalf("G1ADD(O, O): %v", err)
	}
	if !bytes.Equal(out, zero) {
		t.Errorf("G1ADD(O, O) = %x, want zero/identity", out)
	}
}

func TestPrecompiledContractsBLS12381_AcceptsIdentityG2(t *testing.T) {
	impl := precompiledContractsBLS12381[BLS12381G2AddAddress]
	zero := make([]byte, g2PointBytes)
	input := append(append([]byte{}, zero...), zero...)
	out, err := impl.Run(input)
	if err != nil {
		t.Fatalf("G2ADD(O, O): %v", err)
	}
	if !bytes.Equal(out, zero) {
		t.Errorf("G2ADD(O, O) = %x, want zero/identity", out)
	}
}

// ---------------------------------------------------------------
// libevm stability canary
//
// If a future libevm upgrade renames or moves the BLS map entries,
// the init() lookup panics at module load. This test catches the
// case where init runs but the wrong addresses are populated.
// ---------------------------------------------------------------

func TestPrecompiledContractsBLS12381_LibevmMapIntact(t *testing.T) {
	required := map[common.Address]string{
		libevmBLSG1AddDraftAddr:      "G1Add",
		libevmBLSG1MultiExpDraftAddr: "G1MultiExp",
		libevmBLSG2AddDraftAddr:      "G2Add",
		libevmBLSG2MultiExpDraftAddr: "G2MultiExp",
		libevmBLSPairingDraftAddr:    "Pairing",
		libevmBLSMapG1DraftAddr:      "MapG1",
		libevmBLSMapG2DraftAddr:      "MapG2",
	}
	for addr, name := range required {
		if _, ok := vm.PrecompiledContractsBLS[addr]; !ok {
			t.Errorf("libevm.PrecompiledContractsBLS lost %s at %s — w3 wiring stale", name, addr.Hex())
		}
	}
}

// itoa for sub-test names without pulling fmt into hot test paths.
func itoa(u uint64) string {
	if u == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[i:])
}
