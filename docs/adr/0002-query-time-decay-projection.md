# ADR 0002: Stateless Query-Time Decay Projections

## Status
Accepted

## Context
Previously, `mark42` attempted to apply memory decay via in-place SQL mutations (`ApplySoftDecay` table scans: `UPDATE observations SET importance = importance * exp(-days/30)`).
As identified in our architectural review (Rich Hickey & Kent Beck), this approach has fundamental flaws:
1. **Complecting Value with Time**: The stored importance score becomes dependent on how many times and at what intervals background cron jobs were invoked. Running decay twice in a short period compounds decay erroneously.
2. **Database Write Contention**: Running full-table write updates locks the database and forces WAL checkpoints, creating write contention with active agent sessions.
3. **Loss of Immutability**: Facts and initial weights should be immutable values, not mutable cells overwritten arbitrarily.

## Decision
1. Treat base importance as an immutable observation property recorded at creation.
2. Calculate effective importance as a pure query-time projection:
   $$\text{EffectiveImportance} = \text{BaseImportance} \times \exp\left(-\frac{\Delta t}{\tau}\right)$$
   where $\Delta t$ is days elapsed since last access (or creation) and $\tau$ is the configured decay constant.
3. Provide `CalculateEffectiveImportance(baseImportance, daysSinceAccess, decayConstant)` as a pure function in `internal/storage/importance.go`.

## Consequences
- **Positive**: Zero write locks during retrieval or decay evaluation.
- **Positive**: 100% idempotent evaluation regardless of evaluation frequency.
- **Positive**: Historical base importance remains intact for audits.
