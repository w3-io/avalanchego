# w3-io fork of ava-labs/avalanchego

This is the w3-io fork of [ava-labs/avalanchego](https://github.com/ava-labs/avalanchego). It exists to hold a single, focused L1 modification: **EIP-2537 BLS12-381 precompiles**, required by the W3 protocol's `Settlement.sol` contract.

The upstream `README.md` describes avalanchego itself. This file describes only what's specific to our fork.

## Scope

This fork's **only** purpose is to enable EIP-2537 BLS12-381 precompiles at addresses 0x0b–0x11 in subnet-evm. That's it.

Anything else lives elsewhere:

| Concern | Lives in |
|---------|----------|
| EIP-2537 precompile wiring | **this fork** (`w3-io/avalanchego`) |
| Manufacturing tooling (stage-api, clock manipulation, proposer-window skip) | `w3-io/staging` patches applied on top of this fork |
| Settlement contracts that consume EIP-2537 | `w3-io/contracts` |
| Production deployment configs | `w3-io/infrastructure` |

Keeping the fork's surface this minimal matters because:

1. **Auditable.** Reviewers see exactly one change against upstream: BLS precompile registration. No manufacturing flags they need to evaluate.
2. **Upstreamable.** When EIP-2537 is ready to push to ava-labs/avalanchego (Linear W3-729), the PR is already in the right shape — no need to strip out unrelated stuff.
3. **Production-shipped without contamination.** Production nodes run this fork's binary directly. Manufacturing-only flags would be a footgun.

If a modification is needed that ISN'T EIP-2537 (or its eventual generalization to standard Ethereum precompiles), the default answer is "patch it in `w3-io/staging`, don't add it here."

## Branch layout

| Branch | Purpose |
|--------|---------|
| `master` | Tracks `ava-labs/avalanchego` master. No w3 changes. Periodically rebased from upstream. |
| `w3/staging` | Working branch. EIP-2537 work lands here as conventional commits. |
| `w3/release` | What production deploys actually run. Cherry-picks from `w3/staging` after testing. |

## Tag scheme

| Tag | Meaning |
|-----|---------|
| `v1.14.0`, `v1.14.1`, ... | Upstream tags, unchanged |
| `v1.14.2-w3.1`, `v1.14.2-w3.2`, ... | Our releases — upstream base + EIP-2537 |

`infrastructure/` deploys pin a specific `vX.Y.Z-w3.N` tag. The first such tag will be cut when EIP-2537 work (Linear W3-713) lands.

## Planned w3 modifications

### EIP-2537 BLS12-381 precompiles — `feat(eip-2537)` (planned, W3-713)

Wires the standard EIP-2537 precompiles at addresses 0x0b–0x11 into subnet-evm's active execution tier so `Settlement.sol` can verify BLS12-381 aggregate signatures on chain.

Avalanche's `libevm` (the geth fork used under subnet-evm) already contains EIP-2537 implementations in a `PrecompiledContractsBLS` map, but they're not wired into the main hardfork tiers. Our patch consists of registering them at the Pectra-final addresses.

See Linear W3-713 for the implementation plan.

## Building

```bash
# Standard upstream build commands work unchanged.
go build -o ./build/avalanchego ./main
go build -o ./build/plugins/subnet-evm ./graft/subnet-evm/plugin
```

## Running locally + testing contracts against the fork

For the full dev loop — build the fork, run a local subnet, deploy `Settlement.sol`, run contract tests against the local L1 — see [`dev/README.md`](dev/README.md).

The end-to-end loop is the only way we verify the fork as a *VM* (rather than just as a compiling Go module). Run it before tagging a new `vX.Y.Z-w3.N` release.

## Rebasing against upstream

When `ava-labs/avalanchego` releases a new version, sync our fork's master then rebase `w3/staging`:

```bash
git checkout master
git pull upstream master
git push origin master

git checkout w3/staging
git rebase master
git push --force-with-lease origin w3/staging
```

If upstream eventually adopts EIP-2537, drop our commit from the branch via interactive rebase.

## CI

`.github/workflows/w3-ci.yml` runs on every push to `w3/**` branches:

- `go build` of avalanchego + the subnet-evm plugin
- `go vet ./...` on `graft/subnet-evm`
- `go test -short` on `graft/subnet-evm/core/vm/...` (where EIP-2537 lands)
- **Divergence check**: any commit on `w3/staging` that touches a file outside the EIP-2537 whitelist fails CI. Keeps the fork's surface intentionally small.

Upstream's full test suite is NOT re-run here — ava-labs already runs that on `master`.

## Why we forked

Industry pattern for production L1 modifications:
- `op-geth` forks `geth`
- `arbitrum-nitro` forks `geth`
- `polygon-bor` forks `geth`

A fork is easier to rebase against upstream than a patch set, easier for auditors to review (line-by-line GitHub UI vs. raw patch files), and easier to upstream (the commits are already in PR-ready form).

Manufacturing-only modifications stay in `w3-io/staging`'s patches/ directory because they MUST NOT ship to production; they're applied dynamically on top of this fork during manufacturing-environment builds.

## Related

- Production deploys: `w3-io/infrastructure` (pins a `vX.Y.Z-w3.N` tag from this repo)
- Settlement contracts that call EIP-2537: `w3-io/contracts`
- Manufacturing patches on top of this fork: `w3-io/staging`
- Upstream collaboration tracking: Linear W3-729
