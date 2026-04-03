# Distribution Strategy

Status: Draft

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

This document outlines a generic packaging strategy for distributing `magi`, `magi-sync`, and related binaries.

## Goals

- easy install for solo users
- predictable package-manager paths for developers
- automation-friendly release flow for maintainers
- enterprise-friendly binaries and checksums

## Packaging Priorities (Generic)

1. release archives with checksums
2. a macOS package-manager formula
3. Linux native packages (`.deb`, `.rpm`, `.apk`)
4. Windows package-manager manifests

## Binary Strategy

### `magi`

Main memory server. Best distributed as:

- release archives
- container images

### `magi-sync`

Local edge sync binary. Best distributed as:

- release archives
- OS-native packages and package-manager formulas

### `magi-import`

Support tool for markdown import. Lower priority for package-manager discoverability.

## Recommendation

Start with signed release archives and checksums for all binaries, then add package-manager formulas and native packages as adoption grows.
