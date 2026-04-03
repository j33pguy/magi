# Strategy Rollup

Status: Draft

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

This document consolidates product, architecture, deployment, and sync direction in one place. It is planning-oriented and not a public promise.

## Core Positioning

MAGI is:

- shared memory for isolated AI agents
- continuity infrastructure for fragile agent sessions
- portable context across machines, agents, and environments
- self-hosted by default, enterprise-capable by design

Short version:

Shared memory and continuity for isolated AI agents.

## The Problems MAGI Solves

- cross-machine continuity
- cross-agent handoffs
- session resilience after resets or context loss
- project rehydration after fresh clones

## Product Principles

- fast on one box first
- scale by role, not by cloning everything
- flexible defaults with clear guidance
- privacy and ownership first

## Deployment Ladder

- Tier 1: single node with SQLite
- Tier 2: single node with PostgreSQL and auth
- Tier 3: role-separated containers and dedicated embedders

## magi-sync Direction

- edge binary for isolated machines
- local-first privacy controls
- push sync first
- machine identity tagging

## Identity And Access Direction

- identity model: `user`, `machine`, `agent`
- access tags: `owner`, `viewer`, `viewer_group`
- enforce access in recall/search/list

## Documentation Priorities

- switch machines without losing context
- recover after resets and shrinking context windows
- let one agent continue another's work
- start small, scale cleanly
