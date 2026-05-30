# Dev loop: running this fork end-to-end

This directory contains scripts and docs for running the fork
locally so contracts can be deployed and tested against it.

## TL;DR

```bash
# 1. Build the fork's avalanchego + subnet-evm plugin and install
dev/build-and-install.sh

# 2. Create + deploy a local subnet using the binary
W3_TAG=$(git describe --tags --abbrev=0 --match='v*-w3.*')
EWOQ=0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC

avalanche blockchain create w3devnet \
  --evm --evm-chain-id 22323 --evm-token W3 \
  --test-defaults --proof-of-authority \
  --validator-manager-owner $EWOQ \
  --proxy-contract-owner $EWOQ \
  --vm-version $W3_TAG --force

avalanche blockchain deploy w3devnet --local \
  --avalanchego-path "$(pwd)/build/avalanchego"

# 3. Capture the RPC URL
export ETH_RPC_URL=$(avalanche blockchain describe w3devnet --local --json \
  | jq -r .rpcUrls[0])
echo "Local w3 devnet: $ETH_RPC_URL"

# 4. From the contracts repo, deploy + test against the devnet
cd ../contracts
forge script script/DeploySettlement.s.sol --rpc-url "$ETH_RPC_URL" --broadcast
forge test --fork-url "$ETH_RPC_URL"     # excluding BLSForkTest until W3-713
```

## Why we need this

The fork is only useful if we can prove the full stack works: the
fork's binary → real Avalanche consensus → our contracts → green
tests. Without this loop, we'd be shipping a binary that's only
been unit-tested in `graft/subnet-evm/core/vm/...` — never
exercised as part of the consensus protocol it's meant to run.

`BLS.fork.t.sol` in the contracts repo currently runs against
Ethereum mainnet. That proves the BLS solidity wrappers *call*
EIP-2537 correctly when EIP-2537 is available. It does NOT prove
EIP-2537 will be available on our L1 — that's what this dev loop
is for, once W3-713 lands.

## Components

### `build-and-install.sh`

Builds two binaries from the current working tree:

1. **subnet-evm plugin** → `~/.avalanche-cli/bin/subnet-evm/subnet-evm-<w3-tag>/subnet-evm`
2. **avalanchego** → `<repo>/build/avalanchego`

Both are needed because avalanche-cli's `--vm-version <tag>` looks
up the subnet-evm plugin in `~/.avalanche-cli/bin/`, and
`--avalanchego-path` points at a specific avalanchego binary. The
versions must match (subnet-evm's RPC version and avalanchego's
RPC version are checked at boot).

### Local subnet via avalanche-cli

The simplest way to run an Avalanche L1 is via
[avalanche-cli](https://docs.avax.network/tooling/cli) (`avalanche
blockchain create ... && avalanche blockchain deploy ... --local`).
It handles the proposer-VM, P-chain, and bootstrap dance for us.

Key flags for a w3 devnet:

- `--vm-version v1.14.2-w3.1` — tells avalanche-cli to use our
  installed subnet-evm binary
- `--avalanchego-path .../build/avalanchego` — use our avalanchego
  instead of the cached upstream one
- `--evm-chain-id 22323` — w3 testnet chain ID (avoid collision
  with upstream defaults; pick something else for prod deploys)

## End-to-end test flow

The full integration test, once W3-713 lands:

```
   ┌─────────────────────────────────────────────────────────┐
   │  GitHub Actions: contracts CI                            │
   │                                                          │
   │  1. checkout contracts                                   │
   │  2. checkout w3-io/avalanchego @ vX.Y.Z-w3.N             │
   │  3. cd avalanchego && dev/build-and-install.sh           │
   │  4. avalanche blockchain create/deploy w3devnet --local  │
   │  5. cd contracts                                         │
   │  6. forge script DeploySettlement.s.sol --broadcast      │
   │  7. forge test --fork-url $LOCAL_RPC                     │
   └─────────────────────────────────────────────────────────┘
```

CI integration is tracked in Linear W3-730 — for now the loop is
run manually before each fork tag release.

## What works today vs. what's blocked on W3-713

| Test surface | Status |
|--------------|--------|
| `forge test` (unit, no fork) | Works today (no L1 needed) |
| Deploy `Settlement.sol` to w3 devnet | Works today (no BLS needed for deployment) |
| `Settlement.sol` admin / UUPS / non-BLS tests against devnet | Works today |
| `BLSForkTest` against w3 devnet | Blocked on W3-713 (EIP-2537 wiring) |
| Production deploy with BLS sig verification | Blocked on W3-713 |

`BLSForkTest` works against Ethereum mainnet today (mainnet has
EIP-2537 since Pectra). Use that as the reference target until
W3-713 wires EIP-2537 into our fork.

## Manufacturing tooling (NOT in this fork)

Manufacturing — running the chain at simulated past timestamps to
build historical L1 state — needs additional VM modifications
(stage-api RPC namespace, proposer-window check skip). Those live
in [`w3-io/staging`](https://github.com/w3-io/staging)'s
`patches/` directory and apply on top of this fork.

Production nodes MUST NOT have those patches applied. They exist
only for the manufacturing pipeline.

## Troubleshooting

**`avalanche blockchain deploy` says vm-version not found**

The fork's `vX.Y.Z-w3.N` tag isn't a published avalanche-cli VM.
Make sure `dev/build-and-install.sh` ran successfully and that the
installed path exists:

```bash
ls -la ~/.avalanche-cli/bin/subnet-evm/subnet-evm-v1.14.2-w3.1/
```

**RPC version mismatch on deploy**

If you see "the current local network uses rpc version 44 but
your blockchain has version 45 and is not compatible", you need
to pass `--avalanchego-path .../build/avalanchego` so avalanche-cli
uses our fork's avalanchego instead of its cached older one. The
script reminds you of the flag.

**BLS fork test reverts with `precompile failed`**

EIP-2537 isn't wired up yet (W3-713). Run BLS tests against
Ethereum mainnet for now:

```bash
forge test --match-contract BLSForkTest \
  --fork-url https://eth.llamarpc.com
```
