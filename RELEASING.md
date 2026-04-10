# Releasing

## Overview

Releases are driven by [tagpr](https://github.com/Songmu/tagpr). The core principle is **"what you tested is what you ship"** — stable releases promote pre-release images without rebuilding.

## Version Scheme

Versions follow SemVer with pre-release candidates:

- **Pre-release**: `v0.2.1-rc.1` — built and tested, tagged manually or by tagpr
- **Stable**: `v0.2.1` — promotes a tested pre-release image (no rebuild)

## Development Flow

```
  PR merged to main
        │
        ▼
  ┌──────────────┐     ┌──────────────────┐
  │ tagpr opens  │────>│ Release PR       │
  │ Release PR   │     │ (bumps VERSION)  │
  └──────────────┘     │                  │
                       └──────────────────┘
```

## Pre-release (RC)

```
  git tag v0.2.1-rc.1 && git push --tags
        │
        ▼
  ┌─────────────┐     ┌──────────────────┐
  │ CI: Build   │────>│ Push images      │
  │ 4 variants  │     │ (GHCR)           │
  │ amd64+arm64 │     │                  │
  └─────────────┘     └──────────────────┘
        │
        ▼
  Images tagged: <sha>, <version-rc.N>
```

## Stable Release

```
  Merge tagpr Release PR
        │
        ▼
  ┌──────────────┐     ┌──────────────────────────────┐
  │ tagpr tags   │────>│ CI: Promote pre-release   │
  │ v0.2.1       │     │ (re-tag, no rebuild)      │
  └──────────────┘     │                           │
                       └───────────────────────────┘
        │
        ▼
  Images tagged: <version>, <major.minor>, latest
```

## Image Tags

Each build produces four multi-arch image variants:

```
ghcr.io/neilkuan/openab-go:<tag>          # kiro-cli
ghcr.io/neilkuan/openab-go-codex:<tag>    # codex
ghcr.io/neilkuan/openab-go-claude:<tag>   # claude
ghcr.io/neilkuan/openab-go-gemini:<tag>   # gemini
```

Tag patterns:
- **Pre-release**: `<sha>`, `<version-rc.N>`
- **Stable**: `<version>`, `<major.minor>`, `latest`

