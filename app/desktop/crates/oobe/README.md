# oobe

## Owns

The first-run screens (brand, system check, adding a first space or a host) and the display of the
system check.

## Public interface

- `OobeView`: `first_run`, `add_host`; it emits `OobeEvent`.
- `OobeScreen`: the screen shown (exported today; only the crate's tests use it).

## Depends on

- `store`: `SystemCheck`, `Tool`, `ToolReport`.
- `ui`: the design system.
- `gpui`.
