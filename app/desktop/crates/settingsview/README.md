# settingsview

Layer L3 (gpui view). See `docs/plans/2026-11-18-rust-tree.md` section 4.6.

## Owns

The GPUI settings dialog drawn over `settings::State`: the frame, header and search field,
the page rail, the scrolling pages with their sections, rows and controls, record lists, the
shortcut capture UI and key routing from GPUI keystrokes to the state machine.

## Public interface

- `ModalConfig`: `new(schema, settings, records)`, then `page(PageId)` and `on_close(handler)`.
- `SettingsModal`: `new(ModalConfig, window, cx)`, `focus`.
- `bindings`: the dialog keys, unbound while a shortcut is captured.

## Depends on

- `settings`: the schema, values, record lists and the state machine the dialog draws.
- `chord`: `Chord::label` and `Primary` for shortcut rows.
- `ui`: the design system (theme, icons, inputs, menus).
- External: `gpui`.
