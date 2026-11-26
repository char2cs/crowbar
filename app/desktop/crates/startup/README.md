# startup

## Owns

The halt gate: once the daemon halts, what replaces the window's content until the daemon is
started again (develop's `daemon-halt-gate.tsx`). Every other daemon state is the shell's
toasts and crash panel.

## Public interface

- `View`: the gate, which asks the shell through `Actions` (`Command`) and says whether it
  `covers()` the window.
- `Input`, `MigrationStep`, `Outage`: what the shell feeds it.

## Depends on

- `ui`: the design system.
- `gpui`.
