# atomicfile

## Owns

Replacing a file all-or-nothing: the bytes are staged in a sibling file with owner-only mode,
synced, renamed over the target, and the directory is synced, so a crash or a full disk never
leaves the target truncated.

## Public interface

- `write(path, bytes)`: replaces the file at `path` with `bytes`.

## Depends on

No workspace crate (tracing).
