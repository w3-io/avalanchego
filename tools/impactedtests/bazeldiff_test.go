// Copyright (C) 2019, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeSRIHash(t *testing.T) {
	t.Parallel()

	decoded, err := decodeSRIHash(bazelDiffHash)
	require.NoError(t, err)
	require.Equal(t, "F7opo1MmvosIVObhv29FA2gq9bBBZfeaPn+96apq5uY=", base64.StdEncoding.EncodeToString(decoded))
}
