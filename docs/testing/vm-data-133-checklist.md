# Issue #133 acceptance audit

Status: **completed on 19 November 2026** by project coordinator decision.
The prerequisite data contract #127 and the minimal Runtime contract #128
were approved without additional individual-owner sign-offs. This is an
acceptance record for the Sprint 4 VM increment, not for the final Map app.

| Issue criterion | Result | Evidence |
| --- | --- | --- |
| Real Compiler + VM E2E on Observation and Target | Passed | `vm_data_e2e` uses real `tacc`, `tacvm` and `tests/vm-data/test_data.py`; `data_tc_e2e` from #165 |
| Type errors, malformed bytecode, input boundaries | Passed for Sprint 4 scope | `vm_instructions`, `vm_value_boundary`, Compiler/Runtime helper tests and data acceptance vectors |
| Two independent VM processes do not share unsafe state | Passed | `vm_data_e2e` starts two OS processes; `vm_contexts` and `vm_value_boundary` cover independent values/threads |
| Float/IPC/hello and sanitizer/CI regressions stay green | Passed | GitHub Actions Build/Lint/Go/Server acceptance E2E succeeded for PR #164 and #165, including Linux CTest; local ASan/UBSan and TSAN checks described in those PRs |

## Implementation boundaries

- PR #165 adds the nine String/JSON helpers and Compiler signatures needed by
  Observation/Target; PR #164 adds VM boundary/value ownership tests.
- Existing `CALL_RUNTIME` and bytecode v1/v2 remain compatible; no GC, heap,
  JSON DOM, new object model or new opcode is added.
- Generated strings are owned `std::string` values with normal C++ lifetimes,
  including repeated replacements. `Float` remains finite across VM/Runtime.
- PR #167 adds `json.validateObservation` and `json.validateTarget`, which
  return a status for bad *domain JSON* without terminating VM. Legacy typed
  getters still terminate with `RuntimeError` on unvalidated bad input.

## Out of scope / still open

- `runtime.ipc_poll` and `runtime.ipc_message` have been approved in the
  #128 design but are **not yet implemented**. The existing blocking receive
  can still fail before returning a malformed IPC frame.
- Final event-loop recovery, stale-map handling, native window close and
  long-running memory/resource budgets belong to Sprint 5 #143/#145.
- The approval of #133 does not demonstrate AI inference, camera-to-Analyzer
  integration or completion of the final Tactical App.

Decision log: issues #127, #128 and #133 were closed as completed on
19 November 2026 by project coordinator authorization. These references
supersede older draft notes requesting owner reviews; they do not claim that
participants submitted personal approvals.
