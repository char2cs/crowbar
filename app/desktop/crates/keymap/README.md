# keymap

## Owns

The shell's shortcut routing: the default chord of every shell command, user overrides, resolving a
keystroke to a command, the chords a pane forwards to the shell, and the fixed `mod+shift+1..9`
project slots.

## Public interface

- `Keymap`: `defaults`, `with_overrides`, `resolve`, `forwarded`.
- `ShellCommand`: the commands the shell serves.
- `project_slot_of_command`, `project_slot_of_stroke`: the project slot shortcuts.

## Depends on

- `protocol`: `KeyChord` and `KeyBinding`, the wire form of forwarded bindings.
- `chord`: `Keystroke` and `Chord`.
