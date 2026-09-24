# Specifications

This directory contains public contracts shared between `tac-os` components.

Specifications describe **what components exchange and what behavior they rely on**.
They should not contain unnecessary implementation details.

Create a specification only when the corresponding contract becomes necessary.

## Specifications

| File | Contract | Status |
| --- | --- | --- |
| [`language.md`](language.md) | `.tc` syntax | draft, pending owner approval |
| [`bytecode.md`](bytecode.md) | Compiler ↔ VM | draft, pending owner approval |
| [`runtime-api.md`](runtime-api.md) | VM ↔ Runtime | draft, pending owner approval |
| [`ipc.md`](ipc.md) | Analyzer ↔ Map | draft, pending owner approval |
| [`broker-protocol.md`](broker-protocol.md) | Client/Target Service ↔ Broker | draft, pending owner approval |
| [`data-model.md`](data-model.md) | Shared domain structures | draft, pending owner approval |
| [`sql.md`](sql.md) | SQL subset for the DBMS | draft, pending owner approval |

Each file was proposed by Architecture & Integration ahead of Sprint 0 to unblock parallel work; the
owner of the corresponding area (see `docs/ai-context/03-team-ownership.md`) reviews and amends it via a
Pull Request once they pick up their Sprint 0 issue. "Draft" here means "may still change", not
"undecided" — components can build against it in the meantime.

## Rules

- Do not design APIs without a concrete project scenario that requires them.
- Keep contracts minimal.
- Avoid implementation-specific details unless they are part of interoperability.
- If a public contract changes, update the corresponding specification in the same Pull Request.
- Update both sides of the affected interface together whenever possible.
- Add or update an integration test for contract changes.
- If a decision is not fixed yet, keep it in `docs/ai-context/02-open-questions.md` instead of inventing a specification.
