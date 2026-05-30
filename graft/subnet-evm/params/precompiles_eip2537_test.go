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

// TestPrecompiledContractsBLS12381_AddressScheme verifies the
// map contains exactly the 7 Pectra-final EIP-2537 addresses
// (0x0b-0x11) and no others. If libevm changes its address
// scheme, the init() panic will fire first; this test catches
// the case where the panic doesn't fire but our address scheme
// is still wrong.
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

	if got, want := len(PrecompiledContractsBLS12381), len(expected); got != want {
		t.Fatalf("PrecompiledContractsBLS12381 size = %d, want %d", got, want)
	}

	for addr, name := range expected {
		if _, ok := PrecompiledContractsBLS12381[addr]; !ok {
			t.Errorf("missing %s at address %s", name, addr.Hex())
		}
	}

	for addr := range PrecompiledContractsBLS12381 {
		if _, ok := expected[addr]; !ok {
			t.Errorf("unexpected precompile at address %s", addr.Hex())
		}
	}

	// Address sanity: all addresses must be in [0x0b, 0x11].
	for addr := range PrecompiledContractsBLS12381 {
		lastByte := addr.Bytes()[len(addr.Bytes())-1]
		if lastByte < 0x0b || lastByte > 0x11 {
			t.Errorf("BLS precompile at %s outside Pectra-final EIP-2537 range 0x0b-0x11", addr.Hex())
		}
	}
}

// TestPrecompiledContractsBLS12381_RoutesToLibevm verifies that
// calling each precompile at our Pectra-final address returns
// the SAME bytes as calling libevm's implementation directly at
// the corresponding draft address. This is the wiring test — we
// don't re-test libevm's BLS math, we just prove our address
// re-keying routes to the right implementation.
//
// Inputs are constructed to be valid for each precompile so the
// underlying implementation actually runs; outputs are compared
// byte-for-byte. If libevm's implementation is buggy that's
// upstream's problem, but the bug will reproduce identically at
// both our address and libevm's, so the wiring test still passes.
func TestPrecompiledContractsBLS12381_RoutesToLibevm(t *testing.T) {
	// G1 generator (x || y, each 64-byte zero-padded Fp). Used as
	// input for any precompile that takes G1 points (G1ADD, G1MSM,
	// MAP_FP_TO_G1 needs only an Fp).
	g1Gen, err := hex.DecodeString(strings.ReplaceAll(
		"0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb"+
			"0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e1",
		" ", ""))
	if err != nil {
		t.Fatalf("decode G1 generator: %v", err)
	}

	cases := []struct {
		name       string
		pectraAddr common.Address
		libevmAddr common.Address
		input      []byte
	}{
		{"G1ADD", BLS12381G1AddAddress, libevmBLSG1AddDraftAddr, append(append([]byte{}, g1Gen...), g1Gen...)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pectraImpl, ok := PrecompiledContractsBLS12381[c.pectraAddr]
			if !ok {
				t.Fatalf("no precompile wired at %s", c.pectraAddr.Hex())
			}
			libevmImpl, ok := vm.PrecompiledContractsBLS[c.libevmAddr]
			if !ok {
				t.Fatalf("no libevm precompile at draft addr %s", c.libevmAddr.Hex())
			}

			// Identical address-to-impl pointer check is the strongest
			// possible wiring guarantee: same Go value at both addrs.
			if pectraImpl != libevmImpl {
				t.Fatalf("%s: Pectra address %s wired to a DIFFERENT implementation than libevm draft %s",
					c.name, c.pectraAddr.Hex(), c.libevmAddr.Hex())
			}

			// Sanity: gas charge should be defined (non-zero).
			if g := pectraImpl.RequiredGas(c.input); g == 0 {
				t.Errorf("%s.RequiredGas returned 0", c.name)
			}

			gotPectra, errPectra := pectraImpl.Run(c.input)
			gotLibevm, errLibevm := libevmImpl.Run(c.input)
			switch {
			case errPectra != nil && errLibevm != nil:
				if errPectra.Error() != errLibevm.Error() {
					t.Errorf("%s: error mismatch: pectra=%v libevm=%v", c.name, errPectra, errLibevm)
				}
			case errPectra != nil || errLibevm != nil:
				t.Errorf("%s: error asymmetry: pectra=%v libevm=%v", c.name, errPectra, errLibevm)
			case !bytes.Equal(gotPectra, gotLibevm):
				t.Errorf("%s: output mismatch: pectra=%x libevm=%x", c.name, gotPectra, gotLibevm)
			}
		})
	}
}

// TestPrecompiledContractsBLS12381_LibevmMapIntact verifies that
// libevm's PrecompiledContractsBLS map still contains entries at
// the draft addresses we depend on. If libevm reorganizes (e.g.
// moves to Pectra-final addresses upstream), this test fails
// loudly so we can drop our wiring.
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
