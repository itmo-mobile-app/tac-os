# VM data interoperability (#133)

## Implementation scope

The VM now transfers finite Float arguments/results across the existing Runtime
call boundary, in addition to Int and String. The Runtime `Float` enumerator, field and factory are already integrated
in #162; this PR adds independent value/ownership and real data acceptance checks. No device,
Broker, window, camera or Compiler implementation is copied into this branch.
Bytecode versions, tags, instructions, arities and wire schemas are unchanged.

The VM rejects NaN/Infinity arriving from a buggy Runtime handler, even if the
handler bypasses the factory. It rejects invalid kind tags in either direction.
Existing Runtime functions keep enforcing their own signatures: permitting a
Float in the transport does not make `runtime.log(Float)` legal.

## String ownership decision proposed for #127

Existing value-owned `std::string` storage is sufficient for the proposed
immutable `.tc` String semantics. Conversion copies text; it never returns a
borrowed JSON substring or pointer into a parser document. C++ destruction,
assignment and move operations reclaim discarded owners:

- Argument vectors are destroyed after `CALL_RUNTIME`.
- `POP` destroys the removed value, and `STORE` releases the old local's text.
- `RET` transfers the result and destroys the callee frame and unused operands.
- An exception unwinds the VM and its vectors, without a global String arena.

This is the existing implementation's ownership model, not a new object layout,
GC or collection proposal. `.tc` has no mutable shared String reference; modifying
a temporary C++ value cannot change another execution's text.

The 65,536-byte input/output and 72-element JSON limits in PR #152 still need
owner approval and enforcement by the real helpers before allocation. They are
not limits enforced by this VM patch. Total live-memory budgeting also remains
an explicit host/runtime contract question; reclamation alone is not a claim
that every hostile program fits a fixed memory budget.

## Recoverable validation proposal, not an invented API

Keep the nine existing proposed helper names/signatures. To support Map's
requirement to keep displaying the last valid state, #127/#128 owners must
agree a status-returning prevalidation operation for an immutable payload,
including exact domain field sets and numeric ranges. It should use the status
model already proposed in #153. After successful validation, getters can read
the same unchanged String. Programming errors may remain RuntimeError.

No validator name/signature, exception opcode, sentinel result or catch/try
syntax is introduced here. Production malformed-input continuation cannot be
claimed until its actual agreed implementation is exercised. A blanket catch
that replaces failed getters with zero/empty text would violate the contracts.

## Tests

`vm_value_boundary` checks all numeric kinds, exact i64 limits and timestamps
above 2^53, signed zero/subnormals, non-finite and invalid kind rejection,
independently owned Strings (including controls/UTF-8 and 64 KiB text), repeated
replacement of 30,000 generated Strings per worker and two independent workers.
Run the same test in separate ASan/UBSan and TSAN builds.

```sh
(cd compiler && ./gradlew test installDist)
cmake -S . -B build -DTACOS_SANITIZE=ON -DTACOS_VM_FLOAT_E2E=ON
cmake --build build --parallel
ctest --test-dir build --output-on-failure
```

Existing `vm_instructions`, `vm_contexts`, Float, IPC and map-projection tests
remain enabled. They are not evidence that unimplemented JSON helpers work.

## Real Observation/Target acceptance gate

The #127 examples now have real helper implementations in the #132 branch.
The corresponding acceptance fixtures in `tests/vm-data` compile with the
**real** selected tacc and run with the **real** tacvm. They verify independent
VM OS processes, exact Observation fields, escaping, Float projection, i64
timestamps and repeated generated String replacement. Negative type tests
must fail compilation. No signature injection, stub helper or replacement
compiler is used.

With the real Compiler and Runtime helpers from PR #165 (#132) integrated:

```sh
cmake -S . -B build -DTACOS_SANITIZE=ON -DTACOS_VM_DATA_E2E=ON \
  -DTACOS_TACC=/absolute/path/to/real-data-compiler/bin/tacc
cmake --build build --parallel
ctest --test-dir build -R 'vm_data_e2e|vm_value_boundary' --output-on-failure
```

The option is on by default in this integration branch: it uses the actual
tacc/tacvm and fails explicitly when the implementation is incomplete.
The complete #133 acceptance checklist remains open until this real E2E and
approved malformed-message/limits tests pass. Do not label this patch as a
complete implementation of all nine unapproved Runtime functions.

## Cross-branch check with Runtime #162

The opt-in tool below archives the selected real Runtime branch into a temporary
copy and overlays this branch's VM. It does not edit either worktree, inject
Compiler signatures or register fake Runtime functions. It configures explicit
host `DeviceOptions`, reads real `location.latitude()` and `orientation.heading()`
through bytecode `CALL_RUNTIME`, stores/loads Float locals, adds the results and
returns Float. A second program checks an exact timestamp above 2^53.

```sh
python3 tests/vm-data/check_device.py origin/feat/runtime-device-134 cmake
```

Additional standard CMake `-D` arguments may follow the executable. The actual
branch checked was `a2d312771bd8128bd38614b05ec68c1ee71e744b`. Its API accepts
read timeouts 1..1000 ms; the fixture uses 100 ms. This check passed under
ASan/UBSan. Three-way merges of the two shared Runtime value files against
that branch also completed cleanly. The documentation changes merge cleanly
with both contract branches #152/#153 as well. This is real Runtime/VM bytecode integration,
not the still-pending Observation/Target source E2E.

## Recorded results and remaining gates

- 67 existing Compiler tests passed; Compiler source was not changed.
- 33/33 CTest checks passed under ASan/UBSan, with Float source E2E explicitly
  enabled. This includes existing IPC, map projection, two-context and boundary
  tests. The Linux-only `camera_runtime_contract` was excluded on macOS.
- `vm_contexts` and `vm_value_boundary` passed under ThreadSanitizer.
- The candidate Observation source is correctly rejected by today's Compiler
  as an unknown helper (`json.arrayObject`); no result is claimed for that E2E.
- Local host: macOS arm64, AppleClang 15, JDK 17. An external uncommitted
  compatibility header was required to compile Linux socket/signal APIs.
  Linux peer-credential checks are unsupported by that header, not faked as
  successful; camera/process tests still require actual Linux CI.
- #127 owner approvals, recoverable schema validation and agreed resource
  limits remain open. #132 and real data-helper implementations are required
  before the complete #133 acceptance can be closed.

## Current integration check

The VM boundary checks and real data E2E run in CI on top of PR #165,
not on an old Runtime branch. Full schema prevalidation and recoverable
errors remain separate design decisions under #127/#128.
