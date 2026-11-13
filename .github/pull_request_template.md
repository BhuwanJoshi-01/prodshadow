## What and why

<!-- What does this change, and what problem does it solve? -->

## Evidence

<!-- Tests added or updated. For SQL behaviour, an integration test against real PostgreSQL. -->

## Checklist

- [ ] The isolation invariants in docs/threat-model.md still hold.
- [ ] Findings state evidence, threshold and confidence; nothing claims a migration is safe.
- [ ] Docs updated (configuration, rules, report schema) where behaviour changed.
- [ ] `make lint test` passes; `make integration` for database-facing changes.
