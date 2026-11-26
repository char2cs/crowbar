# chord

Layer L0 (logic). See `docs/rust-style.md` (crate layers).

## Owns

The one chord notation shared by the settings page, the shell keymap and the pane script: the
canonical `mod+shift+alt+key` grammar, its parser and its label, which key plays `mod` on this
platform, and the key press the windowing layer reports, free of any toolkit type.

## Public interface

- `Chord`: `parse` (normalises text, `None` for a keyless chord or a repeated modifier), `as_str`
  (the stored text), `label(Primary)` (`Ctrl+Shift+T` or `⇧⌘T`).
- `Primary`: `Control` (Linux) or `Platform` (macOS); `Primary::native()`.
- `Keystroke` (`modifiers`, `key`) and `chord()`, its chord text in the same grammar.
- `Modifier` and `Modifiers`: the modifier keys a keystroke holds.

## Depends on

Nothing: no workspace crate and no external crate.
