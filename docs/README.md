# docs

- [`adapters.md`](adapters.md) — how adapters work, what lives where, built-in adapters, adding new ones
- `adr/` — architecture decision records
- `protocol/` — API and metadata contracts
- `plans/` — execution plans and migration sequencing

## ADRs

| # | Title | Status |
|---|-------|--------|
| 0001 | Architecture split (gmuxr + gmuxd) | Accepted |
| 0002 | gmuxr ownership (launcher + metadata) | Accepted |
| 0003 | Web-first distribution (no Electron v1) | Accepted |
| 0004 | Integrated PTY + WebSocket transport | Accepted |
| 0005 | Runner-authoritative state + adapter system | Accepted |
| 0006 | Information hierarchy and folder probes | Proposed |
| 0007 | Session lifecycle and close semantics | Proposed |
| 0008 | Pi adapter resume and session correlation | Exploring |
| 0009 | Session file attribution via content similarity | Proposed |

## Protocol

- `protocol/gmuxd-rest-v1.md` — gmuxd REST API
- `protocol/session-schema-v2.md` — session metadata model

## Plans

- `plans/mvp-plan.md` — MVP analysis and tiered checklist
- `plans/distribution-v1.md` — binary distribution strategy
- `plans/versioning-release-policy.md` — versioning approach
- `plans/migration-plan-v1.md` — migration from agent-cockpit
