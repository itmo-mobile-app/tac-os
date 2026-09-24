---
name: architecture-change
description: Use when changing component boundaries, public interfaces, shared data models, protocols, bytecode contracts, or Runtime APIs.
---

# Architecture change workflow

Use this skill whenever a change affects an interface between two or more `tac-os` components.

Examples include:

- Compiler ↔ VM bytecode changes;
- VM ↔ Runtime call changes;
- Runtime ↔ Window Manager API changes;
- Analyzer ↔ Map IPC changes;
- Client ↔ Broker protocol changes;
- Broker ↔ Target Service message changes;
- Target Service ↔ DBMS interaction changes;
- shared `Observation`, `Target`, or other domain model changes.

## Before changing anything

1. Read `AGENTS.md`.
2. Read `docs/ai-context/01-architecture.md`.
3. Read `docs/ai-context/02-open-questions.md`.
4. Read the relevant files from `spec/`.
5. Identify all components affected by the proposed change.

## Rules

- Do not change only one side of a shared contract.
- Do not silently introduce incompatible behavior.
- Update the relevant specification before or together with the implementation.
- Keep the contract minimal and driven by an existing project scenario.
- Avoid adding fields, commands, instructions, or API methods for hypothetical future use.
- If the change requires resolving an open architectural question, surface that decision explicitly.
- After a decision is accepted, remove the resolved item from `docs/ai-context/02-open-questions.md`.
- Update relevant integration tests in the same Pull Request.

## Required output for a contract change

Before implementation, describe:

1. what contract is changing;
2. why the current contract is insufficient;
3. which components are affected;
4. the new contract;
5. compatibility impact;
6. how the change will be tested.

## Compatibility

Prefer backward-compatible changes when they do not add unnecessary complexity.

If a breaking change is required:

- update all affected components in the same Pull Request when possible;
- update the specification;
- update tests;
- document the break explicitly in the Pull Request description.

## Definition of done

An architecture-sensitive change is complete only when:

- the specification matches the implementation;
- both sides of the interface are updated;
- relevant project context is updated if necessary;
- resolved open questions are cleaned up;
- integration tests cover the changed interaction.
