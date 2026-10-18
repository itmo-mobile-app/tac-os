# VM and Runtime

Status: draft

This document explains the execution model of `tac-os` and the boundary between the VM and the Runtime.

## Execution pipeline

Application source code is compiled before execution:

```text
source.tc
   ↓
Compiler
   ↓
program.bc
```

The resulting bytecode is executed by the VM:

```text
program.bc
 ↓
VM
 ↓
Runtime
 ↓
Linux
```

The compiler does not need to be present on the client device at runtime.

## VM responsibility

The VM executes the custom `.bc` bytecode.

Its responsibilities include:

- loading bytecode;
- maintaining the instruction pointer;
- maintaining the operand stack;
- maintaining the call stack;
- managing local variables;
- managing heap objects;
- executing arithmetic and logical instructions;
- executing branches and function calls;
- invoking Runtime functions.

The VM should remain independent of Linux-specific implementation details whenever possible.

Examples of VM-level operations:

```text
PUSH
LOAD
STORE
ADD
SUB
JMP
CALL
RET
```

The exact instruction set is not fixed yet.

## Runtime responsibility

The Runtime exposes high-level operations to programs running inside the VM.

It acts as an abstraction layer over Linux system facilities and platform services.

Examples of Runtime functionality:

- threads;
- IPC;
- camera access;
- location;
- device orientation;
- networking;
- files;
- time;
- Activity lifecycle;
- WebView bridge;
- Window Manager access.

A Runtime function may internally use Linux system calls, system libraries, device APIs, or other platform components.

## Runtime calls

A program written in `.tc` may conceptually call:

```text
camera.detectMarkers()
location.get()
ipc.send(...)
thread.start(...)
window.create()
```

The compiler translates such calls into bytecode that identifies a Runtime function.

The VM then dispatches the call to the Runtime.

Conceptually:

```text
.tc code
   ↓
bytecode Runtime call
   ↓
VM
   ↓
Runtime implementation
   ↓
Linux / system component
```

The exact Runtime call ABI is not fixed yet.

## Camera API and private detector

The planned public platform entry point for Analyzer is `camera.detectMarkers()`. The C++ Runtime
delegates recognition to a private Go helper process over a local Unix-domain socket. The helper
owns camera capture and OpenCV-based marker detection; only marker ids, angles, and distances
cross this IPC boundary, not camera frames.

Runtime starts the private Go helper lazily, reuses its socket connection, and owns
its shutdown. For each `DETECT` request the helper returns a JSON array of
`id`, `relativeAngle` and `distance` in a success envelope, or a distinct
camera/processing error. See `spec/camera-detector-protocol.md` for JSONL
framing, error codes, bounded timeouts and lifecycle.

The initial public call `camera.detectMarkers() -> String` returns only the
JSON array (or reports a Runtime error). This temporarily avoids changing
the current VM value layout. Native `Float` and a typed `Detection[]`
return type remain later work; neither the public function nor its IPC
implementation is present in the current executable yet.

The detector is not a third Tactical App process: Analyzer and Map remain independent `.tc`/VM
processes and exchange application observations via `spec/ipc.md`. The Analyzer computes target
coordinates and builds observations; no application business logic lives in the detector.

## Physical deployment

VM and Runtime are separate architectural responsibilities, but they do not need to be separate operating-system processes.

A practical implementation may use one executable:

```text
our-vm program.bc
```

Internally:

```text
our-vm
├── VM Core
└── Runtime
```

The Runtime may be linked directly into the VM executable.

## Multiple application processes

Analyzer and Map are separate processes of one Tactical App.

Each process runs its own VM instance:

```text
Analyzer process
└── VM instance
    └── Runtime

Map process
└── VM instance
    └── Runtime
```

The processes do not share VM state.

## Threads

Threads provide concurrency inside one process.

Conceptually:

```text
one process
├── thread A
├── thread B
└── thread C
```

Threads belong to the Runtime API because the Runtime maps them to Linux threading facilities.

The exact threading API and synchronization primitives are not fixed yet.

Threads and IPC must remain distinct concepts:

- threads operate inside one process;
- IPC connects separate processes.

## IPC

IPC allows separate processes to exchange data.

Primary project scenario:

```text
Analyzer process
   ↓
IPC
   ↓
Map process
```

From application code, IPC is accessed through the Runtime API.

Conceptually:

```text
Analyzer .tc code
   ↓
VM
   ↓
Runtime IPC
   ↓
Linux IPC primitive
   ↓
Runtime IPC
   ↓
VM
   ↓
Map .tc code
```

The exact Linux IPC primitive and serialization format are not fixed yet.

## Window Manager access

The Window Manager is not part of the VM Core.

Application code accesses it through the Runtime:

```text
.tc application
   ↓
VM
   ↓
Runtime Window API
   ↓
Window Manager
   ↓
Linux graphics/windowing
```

## Activity lifecycle

Activity lifecycle support belongs to the Runtime/framework layer.

The Runtime is expected to handle lifecycle transitions and invoke application callbacks.

Concrete Activities belong to `.tc` application code.

The exact lifecycle is not fixed yet.

## WebView

WebView is exposed through the Runtime.

Conceptually:

```text
.tc code
   ↓
VM
   ↓
Runtime WebView API
   ↓
WebView bridge
   ↓
HTML / CSS / JavaScript
```

The Runtime hides the concrete WebView engine from application code.

## Design rules

- VM Core should not directly depend on camera, GPS, networking, or UI implementation details.
- Runtime should not contain application business logic.
- Threads belong to Runtime, not to application-specific code.
- IPC is a Runtime capability for communication between processes.
- Window Manager remains a separate system component.
- Runtime APIs should be added only when required by application scenarios.
- Keep Runtime calls minimal and stable once shared by multiple components.
