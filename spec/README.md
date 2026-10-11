# Specifications

This directory contains public contracts shared between `tac-os` components.

Specifications describe **what components exchange and what behavior they rely on**.
They should not contain unnecessary implementation details.

Create a specification only when the corresponding contract becomes necessary.

## Specifications

| File | Contract | Status |
| --- | --- | --- |
| [`language.md`](language.md) | `.tc` syntax | approved |
| [`bytecode.md`](bytecode.md) | Compiler ↔ VM | approved |
| [`runtime-api.md`](runtime-api.md) | VM ↔ Runtime | approved |
| [`ipc.md`](ipc.md) | Analyzer ↔ Map | approved |
| [`broker-protocol.md`](broker-protocol.md) | Client/Target Service ↔ Broker | approved |
| [`data-model.md`](data-model.md) | Shared domain structures | approved |
| [`sql.md`](sql.md) | SQL subset for the DBMS | approved |

Each file was drafted by Architecture & Integration and then reviewed and approved by the owner of the
corresponding area (see `docs/ai-context/03-team-ownership.md`) during Sprint 0. An approved contract
changes only through a Pull Request that follows the "Contract changes" rules in `AGENTS.md`.

## Rules

- Do not design APIs without a concrete project scenario that requires them.
- Keep contracts minimal.
- Avoid implementation-specific details unless they are part of interoperability.
- If a public contract changes, update the corresponding specification in the same Pull Request.
- Update both sides of the affected interface together whenever possible.
- Add or update an integration test for contract changes.
- If a decision is not fixed yet, keep it in `docs/ai-context/02-open-questions.md` instead of inventing a specification.
