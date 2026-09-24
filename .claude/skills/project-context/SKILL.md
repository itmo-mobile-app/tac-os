---
name: project-context
description: Load and follow the tac-os project context before making architectural or implementation decisions.
---

# Project context

Before making architectural or implementation decisions in `tac-os`:

1. Read `AGENTS.md`.
2. Read `docs/ai-context/00-project-overview.md`.
3. Read `docs/ai-context/01-architecture.md`.
4. Read `docs/ai-context/02-open-questions.md`.
5. Read the relevant files from `spec/`, if they exist.

## Rules

- Follow the documented architecture.
- Do not introduce new subsystems, protocols, or abstractions unless they are required by an explicit project scenario.
- Prefer the smallest implementation that satisfies the current requirement.
- Do not silently resolve items listed in `docs/ai-context/02-open-questions.md`.
- If implementation requires an unresolved decision, surface it explicitly.
- When a decision is accepted, update the relevant architecture or specification file and remove the resolved item from `02-open-questions.md`.
- Keep component boundaries intact even when one developer owns multiple components.
- Keep documentation consistent with the implementation in the same Pull Request.

## Important architectural boundaries

- Compiler translates `.tc` source into `.bc` bytecode.
- VM executes `.bc` bytecode.
- Runtime exposes high-level system functionality to code running in the VM.
- Window Manager is a separate system component accessed through Runtime APIs.
- Analyzer and Map are separate processes of one Tactical App and communicate through IPC.
- Broker transports messages and contains no target business logic.
- Target Service contains target-domain business logic.
- DBMS executes SQL and manages persistent storage without knowing target-domain semantics.

