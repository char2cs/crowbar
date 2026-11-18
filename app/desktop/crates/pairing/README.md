# pairing

## Owns

The Add Host dialog over a `store::PairingSession`: name and address, the host's fingerprint and
the one-time code.

## Public interface

- `PairingView`: the dialog; it emits `PairingEvent` (`Paired`, `Close`).

## Depends on

- `store`: `PairingSession`, `PairingState`, `PairingStage`, `PairingPending`.
- `ui`: the design system.
- `gpui`.
