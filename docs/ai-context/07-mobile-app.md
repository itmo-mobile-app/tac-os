# Mobile application

Status: draft

This document describes the current client-side application model of `tac-os`.

The user interacts with one Tactical App.

Internally, the application is split into separate processes so that IPC is used in a real application scenario.

## Process model

The current model uses at least two application processes:

```text
Tactical App
├── Analyzer process
└── Map process
```

Each process runs its own VM instance and Runtime state.

Conceptually:

```text
Analyzer process
└── VM
    └── Runtime

Map process
└── VM
    └── Runtime
```

The two processes communicate through the project's IPC mechanism.

## Analyzer process

The Analyzer is responsible for producing local target observations.

Its responsibilities include:

- accessing camera frames through the Runtime;
- detecting a recognizable target marker;
- determining the target identifier;
- estimating recognition confidence;
- estimating target direction;
- estimating target distance;
- reading device position;
- reading device orientation;
- calculating target coordinates;
- creating an `Observation`;
- sending local results to the Map process through IPC.

The Analyzer should not contain shared target aggregation logic.

It should not calculate the final system-wide target probability.

## Map process

The Map process owns the main application UI.

Its responsibilities include:

- displaying a simple 2D tactical map;
- displaying target markers;
- displaying target probability;
- receiving local observations from Analyzer through IPC;
- publishing observations to the central system;
- receiving aggregated target updates;
- updating the visible map state.

The exact UI structure is not fixed yet.

## IPC scenario

The primary local IPC scenario is:

```text
Analyzer
   ↓
TARGET_DETECTED / Observation
   ↓
IPC
   ↓
Map
```

The exact message name, payload format, serialization, and delivery behavior are not fixed yet.

The purpose of this split is to demonstrate communication between separate processes of one application.

## Central communication

The Map process currently acts as the client-side integration point with the central system.

Conceptually:

```text
Analyzer
   ↓ IPC
Map
   ↓ publish Observation
Broker
```

Aggregated target updates return through:

```text
Broker
   ↓ TargetUpdated
Map
   ↓
2D map
```

The exact network transport and Broker protocol are not fixed yet.

## Target coordinates

The client calculates target coordinates using:

- current device coordinates;
- device orientation;
- target direction relative to the device/camera;
- estimated target distance.

The intended project scenario uses relatively short distances, so the coordinate calculation may remain intentionally simple.

The exact calculation method is not fixed yet.

## Target identification

Targets are expected to have recognizable markers or identifiers.

The initial design avoids complex target matching.

If multiple clients detect the same target identifier, the central system treats the reports as observations of the same physical target.

## UI

The UI is intended to remain minimal.

The main required visual feature is a simple 2D tactical map containing target markers.

A WebView may be used to simplify implementation.

Possible model:

```text
.tc application logic
   ↓
Runtime WebView API
   ↓
WebView bridge
   ↓
HTML / CSS / JavaScript
   ↓
2D map UI
```

The exact WebView implementation is not fixed yet.

## Activity model

Concrete Activities belong to application code.

Possible Activities include:

- Map Activity;
- Analyzer Activity or Analyzer-related UI.

The actual camera analysis may continue in the separate Analyzer process regardless of which Activity is currently visible.

The exact Activity lifecycle and navigation model are not fixed yet.

## Local and shared target state

The application may temporarily display a locally detected target before the central system responds.

The shared authoritative state is produced by the Target Service and returned as aggregated `Target` updates.

This distinction allows:

- immediate local feedback;
- later replacement or correction using shared state.

The exact behavior is not fixed yet.

## Failure handling

Behavior for the following cases is still open:

- Broker unavailable;
- Target Service unavailable;
- Analyzer process unavailable;
- Map process restarted;
- IPC connection lost;
- stale target data;
- missing camera or location data.

These cases should remain simple unless the final project scenario requires more robust handling.

## Design rules

- Analyzer and Map are processes of one application, not separate products.
- Analyzer produces observations; it does not own shared target state.
- Map owns the user-facing tactical view.
- Shared probability is calculated by the central Target Service.
- IPC is used for local process-to-process communication.
- Broker communication is used for client-to-central-system messaging.
- UI implementation should remain minimal.
- Do not add complex map, navigation, background-service, or offline-sync behavior unless required by the project scenario.
