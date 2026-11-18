# diagnostics

## Owns

The shell's own rotating log, the crash files left beside it, and the support bundle that zips
logs and crash dumps for a bug report.

## Public interface

- `start`: installs the tracing subscriber and the crash hook.
- `LogPaths`: where the shell log and crash files live.
- `Bundle`: zips the logs and dumps into a support bundle.
- `Error`.

## Depends on

No workspace crate (tracing-subscriber, zip).
