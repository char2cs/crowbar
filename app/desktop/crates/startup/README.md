# startup

## Owns

What the user sees until the daemon serves: the full-screen startup and migration view, and its
gpui-free state machine (phases, timing, migration steps, outages and halts).

## Public interface

- `StartupView`: the view, which emits `StartupEvent` and asks the shell through `StartupActions`
  (`StartupCommand`); it reads time through the `StartupClock` port (`MonotonicClock` in
  production).
- `StartupInput`, `StartupTiming`, `AppPhase`, `MigrationStep`, `Outage`, `HaltReason`: what the
  shell feeds it.
- `StartupMachine`, `StartupScreen` (exported today; only the crate uses them).
- `startupgallery` binary (`src/bin/gallery/`): every phase of the view, or a demo run.

## Depends on

- `ui`: the design system.
- `gpui`.
