# Activity Lifecycle Contract

**Status:** Reviewed by Runtime owner.

## 1. Purpose

This document defines the minimal lifecycle contract for Activities in TAC-OS.

It describes:
- the main stages of an Activity lifecycle;
- responsibilities of Activity, Runtime, and Window Manager (WM);
- Activity creation and termination;
- handling of native window close events;
- error-handling expectations.

## 2. Responsibilities

### 2.1. Activity

An Activity is responsible for:
- initializing its application-specific state;
- executing its application-specific logic;
- stopping its work when termination is requested;
- releasing application-specific resources.

An Activity does not directly control the lifecycle of the WM process.

### 2.2. Runtime

Runtime coordinates the Activity lifecycle and is responsible for:
- initiating Activity creation;
- coordinating Activity initialization;
- managing communication with WM;
- handling WM responses and events;
- coordinating Activity termination and resource cleanup;
- ensuring that an Activity does not continue running after its termination.

### 2.3. Window Manager

WM is responsible for native window management.

The WM contract defines:
- `CREATE` — create a native window;
- `SHOW` — display a window;
- `CLOSE` — close a specified window and release its resources;
- `QUIT` — request WM process shutdown;
- `window_close` — notify the client that the user requested window closure through the native window controls.

WM destroys the native window before emitting the `window_close` event.

An application must not send a duplicate `CLOSE` command for a window that WM has already closed.

Activity termination does not request WM process shutdown through `QUIT`.

## 3. Activity States

An Activity has the following lifecycle states:

| State | Description |
|---|---|
| `CREATED` | The Activity has been created and is undergoing initialization. |
| `ACTIVE` | Initialization and required window setup have completed successfully. |
| `STOPPING` | The Activity is terminating and releasing its resources. |
| `DESTROYED` | The Activity has completed termination. |

The lifecycle transitions are:

`CREATED → ACTIVE → STOPPING → DESTROYED`

If initialization fails, the Activity does not transition to `ACTIVE`.

## 4. Lifecycle Flow

### 4.1. Creation and activation

The Activity creation and activation sequence is:

1. Runtime initiates Activity creation.
2. Activity initializes its application-specific state.
3. Runtime requests native window creation from WM using `CREATE`.
4. Runtime may issue additional WM commands required to prepare the window before displaying it.
5. Runtime requests window visibility using `SHOW` when the Activity is ready to be displayed.
6. The Activity enters the `ACTIVE` state when its initialization and required window setup have completed successfully.

An Activity is not considered active if required initialization or window setup fails.

### 4.2. Graceful shutdown

The Activity shutdown sequence is:

1. Runtime or the application requests Activity termination.
2. Activity stops its application-specific work.
3. Runtime requests window closure from WM using `CLOSE`.
4. WM releases the native window resources.
5. Runtime coordinates the release of remaining Activity resources.
6. Runtime marks the Activity as `DESTROYED`.

If the native window has already been closed by WM, Runtime does not send another `CLOSE` command for that window.

### 4.3. Native window close

When the user closes the native window:

1. WM closes the native window and releases its resources.
2. WM emits a `window_close` event containing the associated `window_id`.
3. Runtime detects the event and initiates the corresponding Activity shutdown.
4. The Activity stops its application-specific work.
5. Runtime coordinates the remaining Activity cleanup and completes its lifecycle.

The native window is destroyed before the `window_close` event is delivered.

Runtime does not send a duplicate `CLOSE` command for a window that WM has already closed.

Closing an Activity does not request WM process shutdown through `QUIT`.

## 5. Error Handling

The following error-handling rules apply:

- Runtime checks the `ok` field in WM command responses.
- A failed `CREATE` or `SHOW` operation prevents the Activity from entering the `ACTIVE` state.
- Communication failures between Runtime and WM are handled without assuming that the requested operation succeeded.
- When Activity termination is requested, Runtime coordinates cleanup even if one of the shutdown operations fails.
- Errors during initialization or shutdown are reported through the Runtime's error-handling mechanism.

## 6. Verification

The Activity lifecycle is verified against the following scenarios:

1. An Activity is created and initialized successfully.
2. An Activity fails during initialization.
3. An Activity successfully creates and displays its native window.
4. An Activity is terminated by Runtime.
5. A user closes the native window.
6. Runtime does not send a duplicate `CLOSE` after receiving `window_close`.
7. Activity termination does not shut down the WM process.
8. WM communication or command execution fails during lifecycle operations.

## 7. References

- `spec/window-manager.md` — Window Manager protocol and native window behavior.
- `spec/map-rendering.md` — Map prototype integration with Window Manager.
- `docs/testing/map-wm.md` — Window Manager integration testing.
