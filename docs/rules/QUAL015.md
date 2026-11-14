# QUAL015 · unpredicted-lock-wait

| Default severity | Evidence |
|---|---|
| info | experiment quality |

## What it detects

In a candidate phase, a session waited on a relation because of the migration (the migration itself waited on it, or a session waited behind the migration), but no migration statement was predicted to lock that relation, not even through the fixture schema. Waits on row versions (`transactionid`) and other non-relation locks are not counted, and the rule is silent when a statement could not be analysed.

## Why it matters

ProdShadow predicts locks on tables a statement does not name from the fixture schema: foreign keys, partitions, views, triggers, rules. A wait on a relation the prediction missed means the schema did not show the connection, usually because the fixture schema lacks a foreign key, trigger or partition that production has, or the lock comes from dynamic SQL. The measurement is unaffected, but `plan`'s conflict prediction and the static rules worked from an incomplete picture, and the fixture probably differs from production in other ways too.

## Example

Flagged:

```sql
-- fixtures/schema.sql, without the foreign key production has
CREATE TABLE invoices (id bigint PRIMARY KEY, customer_id bigint NOT NULL);
```

Preferred:

```sql
CREATE TABLE invoices (
    id bigint PRIMARY KEY,
    customer_id bigint NOT NULL REFERENCES customers (id)
);
```

## How ProdShadow verifies it

Each candidate phase lists the relations sessions waited on (`phases[].locks.relations`), with `migration: true` when the migration was involved (since report schema 1.9.0); compare them with the predicted locks in `migration.statements`.

## Configuration

Change the severity or turn the rule off in `prodshadow.yaml`:

```yaml
policy:
  rules:
    QUAL015: critical   # or info, warning, off
```

Suppress a reviewed case in the suppressions file (`policy.suppressions`):

```yaml
version: 1
suppressions:
  - rule: QUAL015
    reason: "why this is acceptable"
    expires: 2027-06-30
```

## References

- [Locks beyond the named table](../guides/locks-beyond-the-named-table.md)
- [ADR 0011](../adr/0011-schema-catalog.md)
- [OBS017](OBS017.md)
