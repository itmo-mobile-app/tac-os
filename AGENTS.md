## Purpose

This file defines the rules for AI agents working with the `tac-os` repository.

`tac-os` is an educational project that implements a simplified Android-like platform on top of Linux.

The mobile application is written in a custom programming language, compiled into `.bc` bytecode, and executed by a custom virtual machine.

## Language convention

- Code, commit messages, `AGENTS.md`, `docs/`, and `spec/` are written in English.
- GitHub Issues and Pull Request discussion (titles, descriptions, comments, review threads) are written in Russian.
- Do not mix languages within the same file or the same PR title/description.

## Before making changes

Before modifying code, read:

1. `README.md`
2. `docs/ai-context/00-project-overview.md`
3. `docs/ai-context/01-architecture.md`
4. `docs/ai-context/02-open-questions.md`
5. relevant files from `spec/`, if they already exist

If a required decision is not documented, do not assume that it has already been made.

## General rules

- Do not change the architecture without an explicit reason.
- Do not add new subsystems or abstractions "for future use".
- Prefer the smallest implementation sufficient for the current scenario.
- Do not replace required custom components with ready-made equivalents.
- Linux is the base operating system and may be used through system APIs.
- The custom language is primarily intended for the mobile application.
- Source files written in the custom language use the `.tc` extension.
- Compiled bytecode files use the `.bc` extension.
- Public contracts between components must be documented in `spec/`.
- If a decision is not fixed yet, treat it as an open question instead of silently choosing an implementation.

## Architectural boundaries

### Compiler

Responsible for:

- lexing and parsing;
- syntax and semantic analysis;
- generating `.bc` bytecode.

Not responsible for executing programs.

### VM

Responsible for:

- loading bytecode;
- instruction pointer;
- operand stack;
- call stack;
- heap;
- instruction execution;
- Runtime API calls.

The VM must not contain application business logic.

### Runtime

Provides high-level APIs to programs running inside the VM.

Runtime functionality includes or exposes access to:

- threads;
- IPC;
- camera;
- location;
- orientation;
- networking;
- files;
- time;
- Activity lifecycle;
- WebView bridge.

The Runtime may be linked into the VM executable instead of running as a separate process.

### Window Manager

A separate system component.

Responsible for:

- creating windows;
- showing and closing windows;
- dispatching input events.

Applications access the Window Manager through Runtime APIs.

### Mobile App

The user sees a single Tactical App.

Internally, the application uses at least two processes:

- `Analyzer`;
- `Map`.

`Analyzer` and `Map` communicate through IPC.

### Broker

Responsible only for message delivery using publish/subscribe.

The Broker must not contain target-related business logic.

### Target Service

Responsible for:

- consuming `Observation` messages;
- aggregating observations;
- calculating `Target` state;
- probability calculation and time decay;
- reading and writing data through the DBMS;
- publishing target updates through the Broker.

### DBMS

Responsible for:

- the supported SQL subset;
- query execution;
- persistent data storage.

The DBMS must not know about Broker semantics or target-domain business logic.

## Contract changes

Before changing an interface between components:

1. identify all affected components;
2. check the existing contract in `spec/`;
3. update the specification;
4. update both sides of the interaction;
5. add or update the corresponding integration test.

Pay special attention to these contracts:

- Compiler ↔ VM;
- VM ↔ Runtime;
- Runtime ↔ Window Manager;
- Analyzer ↔ Map;
- Client ↔ Broker;
- Broker ↔ Target Service;
- Target Service ↔ DBMS.

## Working on tasks

Do not attempt to implement an entire large component unless necessary.

Prefer small, testable increments.

Bad:

> Implement the VM.

Better:

> Make the VM load `.bc` bytecode and execute the minimal instruction set required for the first compiler → VM integration test.

## Definition of done

A change is considered complete when:

- it follows the documented architecture;
- public contracts are updated if necessary;
- there is a clear way to verify the result;
- existing integration scenarios remain working.

## Repository workflow

### Issues

- Every non-trivial change should be associated with a GitHub Issue.
- Before starting implementation, check whether an Issue already exists.
- If no suitable Issue exists, create one before making significant changes.
- Keep Issues small and focused on one concrete increment.
- An Issue should describe a verifiable result, not only a component name.

Bad:

> Implement Runtime.

Better:

> Add `ipc.send` and `ipc.receive` to the Runtime and cover communication between Analyzer and Map with an integration test.

### Branches

- Do not work directly on `main`.
- Create a short-lived branch for each Issue.
- Prefer names such as:
    - `feat/runtime-ipc`
    - `feat/vm-loader`
    - `fix/broker-subscription`
    - `docs/runtime-api`

### Pull Requests

- Changes should reach `main` through a Pull Request.
- Keep Pull Requests focused and reasonably small.
- A Pull Request should reference the related Issue.
- Update affected specifications and tests in the same Pull Request when possible.

### Commits

Use Conventional Commits.

Preferred types:

- `feat:` — new functionality;
- `fix:` — bug fix;
- `docs:` — documentation only;
- `refactor:` — internal change without changing behavior;
- `test:` — tests;
- `chore:` — repository/build/tooling changes;
- `ci:` — CI configuration.

Examples:

```text
feat(vm): add bytecode loader
feat(runtime): add ipc send and receive
fix(broker): remove stale subscribers
docs(spec): define runtime call ABI
test(vm): add function call integration test
chore(repo): add initial project structure
```
Commit messages should describe the actual change and remain concise.

## Documentation maintenance

Project context files are living documents and must be kept up to date.

When a change affects architecture, component responsibilities, public contracts, or previously open decisions, update the relevant documentation in the same Pull Request.

Rules:

- Update `docs/ai-context/00-project-overview.md` only when the overall project model changes.
- Update `docs/ai-context/01-architecture.md` when architectural decisions or component boundaries change.
- Update `docs/ai-context/02-open-questions.md` when:
  - a new unresolved architectural question appears;
  - an existing question is resolved.
- When an open question is resolved, remove it from `02-open-questions.md` and document the accepted decision in the relevant architecture or specification file.
- Update files in `spec/` whenever a public contract between components changes.
- Do not leave documentation knowingly inconsistent with the implementation.

Documentation changes should be included in the same Pull Request as the code change that caused them.

### Pull Requests

- Changes should reach `main` through a Pull Request.
- Keep Pull Requests focused and reasonably small.
- Every Pull Request should reference and close the related Issue.
- Use GitHub closing keywords in the Pull Request description, for example:
  - `Closes #12`
  - `Fixes #12`
  - `Resolves #12`
- Pull Request titles must follow this format:

```text
Type | Short description
```

- `Type` must be capitalized (e.g. `Feat`, `Fix`, `Docs`, `Refactor`, `Test`, `Chore`, `CI`).

- A task is not complete until its Pull Request is opened; opening the PR is the final step of every task.
- The Pull Request must be opened from the branch of the agent/developer who performed the task, under that same agent's/developer's account.
- The Pull Request description must be filled in (not left as a template with empty sections).
- When opening a Pull Request, set: labels (component/type of change), assignees (the agent/developer who did the work), and the relevant GitHub Project (if the repository uses one), instead of leaving these fields empty.
