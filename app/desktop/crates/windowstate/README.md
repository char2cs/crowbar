# windowstate

## Owns

The shell window remembered between runs: the window state file, and a placement that is visible on
a display that is still connected.

## Public interface

- `WindowStore`: loads and saves the window state through its `StateFile` port.
- `WindowState`, `Placement`, `Rect`, `MIN_SIZE`, `DEFAULT_SIZE`: what is remembered.
- `Error`.

## Depends on

- `atomicfile`: the state file is replaced all-or-nothing.
