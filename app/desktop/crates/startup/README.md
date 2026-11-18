# startup

## Owns

What the user sees until the daemon serves: the full-screen startup and migration view, and its
gpui-free state machine (phases, timing, migration steps, outages and halts).

## Public interface

- `View`: the view, which emits `Event` and asks the shell through `Actions`
  (`Command`); it reads time through the `Clock` port (`MonotonicClock` in
  production).
- `Input`, `Timing`, `AppPhase`, `MigrationStep`, `Outage`, `HaltReason`: what the
  shell feeds it.
- `startupgallery` binary (`src/bin/gallery/`): every phase of the view, or a demo run.

## Depends on

- `ui`: the design system.
- `gpui`.
