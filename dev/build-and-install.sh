#!/usr/bin/env bash
#
# build-and-install.sh — build the w3 subnet-evm plugin from the
# current checkout and install it where avalanche-cli expects to
# find it. After this script runs, avalanche-cli launches subnets
# with our fork's binary instead of the upstream-bundled one.
#
# Idempotent — re-running rebuilds and reinstalls under the same
# w3-tagged directory.
#
# Usage:
#   dev/build-and-install.sh
#
# Required tools:
#   - go (matching the version in go.mod)
#   - avalanche-cli (https://docs.avax.network/tooling/cli)
#

set -o errexit
set -o nounset
set -o pipefail

readonly REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

# Pick the right destination directory inside ~/.avalanche-cli.
# avalanche-cli versions its plugins by subnet-evm release tag.
# We install under our own w3-tagged directory so the upstream
# binary at the standard path is not touched.
# Resolve the w3 tag from (in order): explicit env override, an
# exact tag on the current commit, then fall back to the closest
# w3 tag. The exact-tag check matters because callers (like the
# contracts repo's w3-devnet.sh) check out a specific tag and
# expect the installed binary to land at the path that tag names.
# If we fell back to `git describe --abbrev=0` here and the user
# was on a commit that wasn't itself tagged, the caller would
# look under one tag's path and find a binary under another's.
if [[ -n "${W3_FORK_TAG:-}" ]]; then
    W3_TAG="${W3_FORK_TAG}"
elif W3_TAG="$(git describe --tags --exact-match --match='v*-w3.*' 2>/dev/null)"; then
    :
else
    echo "error: current HEAD is not on a v*-w3.* tag and W3_FORK_TAG is not set." >&2
    echo "       Either checkout a tagged commit or pass W3_FORK_TAG=v1.14.2-w3.N" >&2
    exit 64
fi
readonly W3_TAG
readonly INSTALL_DIR="${HOME}/.avalanche-cli/bin/subnet-evm/subnet-evm-${W3_TAG}"
readonly INSTALL_BIN="${INSTALL_DIR}/subnet-evm"

if ! command -v go >/dev/null 2>&1; then
    echo "error: go not found on PATH (brew install go)" >&2
    exit 127
fi

if ! command -v avalanche >/dev/null 2>&1; then
    echo "warn: avalanche-cli not found on PATH — installing the" >&2
    echo "      binary anyway, but you'll need to install avalanche-cli" >&2
    echo "      before you can launch a network with it." >&2
fi

echo "w3-fork build: tag=${W3_TAG}"
echo "w3-fork build: source=${REPO_ROOT}"

cd "${REPO_ROOT}/graft/subnet-evm"

echo "w3-fork build: compiling subnet-evm plugin (~30s)"
go build \
    -o "/tmp/w3-subnet-evm-${W3_TAG}" \
    ./plugin

mkdir -p "${INSTALL_DIR}"
cp "/tmp/w3-subnet-evm-${W3_TAG}" "${INSTALL_BIN}"
chmod +x "${INSTALL_BIN}"

echo ""
echo "w3-fork build: subnet-evm installed"
echo "         binary: ${INSTALL_BIN}"
echo "         size:   $(du -h "${INSTALL_BIN}" | cut -f1)"
echo "         tag:    ${W3_TAG}"
echo ""

# Build avalanchego too. Production needs only the subnet-evm
# plugin, but for the dev loop we also need avalanchego itself so
# avalanche-cli can boot a network with --avalanchego-path.
echo "w3-fork build: compiling avalanchego (~60s)"
cd "${REPO_ROOT}"
go build -o "${REPO_ROOT}/build/avalanchego" ./main
echo "w3-fork build: avalanchego installed"
echo "         binary: ${REPO_ROOT}/build/avalanchego"
echo "         size:   $(du -h "${REPO_ROOT}/build/avalanchego" | cut -f1)"
echo ""
echo "Next:"
echo "  1. Create + deploy a local subnet:"
echo "       avalanche blockchain create w3devnet \\"
echo "         --evm --evm-chain-id 22323 --evm-token W3 \\"
echo "         --test-defaults --proof-of-authority \\"
echo "         --validator-manager-owner 0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC \\"
echo "         --proxy-contract-owner 0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC \\"
echo "         --vm-version ${W3_TAG} --force"
echo ""
echo "       avalanche blockchain deploy w3devnet --local \\"
echo "         --avalanchego-path ${REPO_ROOT}/build/avalanchego"
echo ""
echo "  2. Point the contracts repo's fork tests at the local RPC:"
echo "       ETH_RPC_URL=\$(avalanche blockchain describe w3devnet \\"
echo "         --local --json | jq -r .rpcUrls[0]) \\"
echo "         forge test --fork-url \$ETH_RPC_URL"
echo ""
echo "Note: BLS fork tests will fail until W3-713 wires EIP-2537"
echo "into this fork's subnet-evm. Non-BLS tests work today."
