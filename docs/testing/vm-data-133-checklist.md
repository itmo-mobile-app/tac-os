# Issue #133 acceptance audit

Reviewed against main `3877600` and the open contract branches #152/#153.
This checklist distinguishes implemented VM behavior from unapproved interfaces.

| Issue criterion | Status | Evidence / next action |
| --- | --- | --- |
| Real new Compiler + VM E2E on Observation and Target | Prepared, blocked | `tests/vm-data/observation.tc`, `target.tc`, `strings.tc`, `test_data.py`; requires approved #127, real helpers and Compiler #132 |
| Type errors, malformed bytecode, input boundaries | VM part verified; helper part pending | `vm_instructions` checks malformed v1/v2, indices/branches/stack/type errors. `vm_value_boundary` checks Float, i64 and String ownership. Proposed JSON/bounds tests require actual helpers |
| Two independent VM processes do not share unsafe state | Existing process scenario verified; new data scenario prepared | `ipc_vm_e2e` runs real separate processes; `vm_contexts` and `vm_value_boundary` additionally exercise independent threaded executions; `vm_data_e2e` starts two data-program processes once dependencies exist |
| Float/IPC/hello and sanitizer/CI regressions stay green | Locally verified; Linux CI requested in Draft PR | 33/33 applicable CTest checks, 67 Compiler tests, ASan/UBSan and two TSAN checks. The real Float source E2E is now on by default after Compiler #107 merged |

## Work completed for each agreed boundary

1. **VM review of #152/#153:** no new opcode/version is necessary for the
   candidate helpers. The Float transport omission is fixed and tested against
   the real Runtime #162. Review text identifies the remaining contract gates.
2. **Float transport:** the shared Runtime fields/factory match #162 exactly;
   both conversion directions preserve kinds and reject NaN/Infinity.
   Three-way merges with that Runtime branch are clean.
3. **Generated Strings:** ownership is by value using existing `std::string`.
   Tests cover independent copies, destruction of source values, repeated
   replacement, controls/UTF-8, large text, threads and existing VM frame paths.
   This supports an RAII decision without a global arena or GC.
4. **Invalid external JSON:** a status-returning prevalidation phase is proposed
   for the same immutable payload. No name, sentinel or catch opcode is chosen
   without the owners. Exact-field-set validation and resource limits remain
   explicit decisions in #127/#128, not hidden implementation choices.
5. **Joint source acceptance:** fixtures use the proposed existing names and
   real tools only. They fail on today's unknown helpers. Negative tests demand
   controlled exit status and diagnostics, never treating a crash as success.
6. **Repository workflow:** one branch for #133, Conventional Commits, Russian
   Draft PR and review bodies, owner assignment/labels, tests and documentation.
   Keep the Draft open until all acceptance rows can be marked passed.

## Repository scope reviewed

The inventory covers all 240 versioned paths in main: 234 text files and six
binary artifacts. Compiler, VM, Runtime, app/Map, Broker, Target Service, DBMS,
Window Manager, marker detector, tests, specifications and build/CI boundaries
were checked for the #133 dependencies. Generated build directories, local
worktrees and unrelated user archives are excluded. A path/hash inventory is
kept in the local handoff folder; binary fixtures are not interpreted as source.

No implementation of the nine unapproved helpers was found in main. No
published Compiler #132 implementation branch was found during the audit. Existing device/Broker/WM PRs are not
rewritten by this work. The Runtime Float common-value changes are the only
shared implementation addition required here.

## Remaining owner actions

- #127/#128 owners: settle recoverable schema validation, exact field sets,
  limits, final helper ownership and review the RAII rule; reconcile the one
  `spec/runtime-api.md` with #156. A documentation merge is not owner approval.
- Compiler #132: register approved helpers and Camera signature, keep type checks.
- Runtime/helper implementation owner: supply real approved functions.
- VM #133: run the prepared real data E2E against those implementations,
  fix genuine interoperability failures and update acceptance evidence.

Do not close #133 on the basis of passing boundary tests or placeholder JSON.
