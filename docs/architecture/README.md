# Architecture diagrams

Status: living document — keep in sync with `docs/ai-context/01-architecture.md` and `spec/`.

The same system is shown on six levels, like the OSI model: each diagram answers one question and does not mix
in details of other levels. The diagrams are embedded in the root `README.md`.

| File | Level | Question | Shows | Does not show |
| --- | --- | --- | --- | --- |
| `1-business.svg` | Business | What does the system do for players? | Actors and scenario steps | Processes, technologies |
| `2-logical.svg` | Logical | Which parts exchange which data? | Components and domain data (`Observation`, `Target`) | Transport, Broker |
| `3-processes.svg` | Processes | What runs where and how is it connected? | Machines, processes, transport, implementation language | Message meaning |
| `4-platform.svg` | Platform | What does the app run on? | Layers: `.tc` app, VM, Runtime, Linux; Window Manager | Network |
| `5-build.svg` | Build | How does source become a program? | Files and tools: `.tc` → `tacc` → `.bc` → `tacvm` | Runtime internals |
| `6-code.svg` | Code | How is one process built inside? | VM and Runtime functions and calls between them | Other processes |

## Notation

| Element | Meaning |
| --- | --- |
| Large rounded frame with a caps label | A machine (player device or central node) |
| Box with a dark header | An OS process; the header is the process name |
| White box | A platform component written by the team |
| Purple box | Application code in `.tc` (runs inside the VM) |
| Small grey box | A function or data structure inside a component (level 6 only) |
| Parallelogram | Input data used by a scenario step (level 1 only) |
| Rounded pill | An actor outside the system: player, team (level 1 only) |
| Sheet with a folded corner | A file |
| Solid arrow | Data flows from start to end; the label says how and what |
| Double-headed arrow | Data flows both ways over one channel |
| Dashed arrow | The result returned by a call (level 6 only) |

Status (done or not) is not shown on the diagrams.

## Keeping the diagrams up to date

The diagrams are plain SVG written by hand: edit text and coordinates directly. Labels are in Russian because the
diagrams are embedded in the Russian `README.md`. Each file starts with a comment that says when it must be updated.

Update the affected diagrams in the same Pull Request as the decision or change that makes them outdated:

| Change | Diagrams to update |
| --- | --- |
| A component language is chosen (replace «Go или Rust») | 3, 4 |
| A transport or protocol is chosen (Runtime ↔ Window Manager, Target Service ↔ DBMS) | 3 |
| A component or process is added, removed, merged, or split | 2, 3, and 4 if it is on the device |
| Domain data exchanged between components changes (`spec/data-model.md`) | 2 |
| Game scenario or rules change (`docs/requirements.md`) | 1 |
| Compiler technology, file formats, or command lines change | 5 |
| The VM ↔ Runtime interface or the load/execute flow changes | 6 |

Keep each diagram on its own level: do not add transport details to level 2 or message meaning to level 3.
