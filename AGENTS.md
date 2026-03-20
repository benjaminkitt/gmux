# AGENTS.md

## State discipline

Never add new state without justification. Before adding a field, ask: who owns it, who updates it, and can it be derived from existing state instead? Prefer derivation over storage. New state creates maintenance burden, sync bugs, and lifecycle complexity.

## Changelog

Commits appear automatically in GitHub Releases. User-facing highlights go in `apps/website/src/content/docs/changelog.mdx`, grouped by version. Keep entries concise and focused on what changed for the user, not implementation details.
