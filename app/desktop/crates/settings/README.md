# settings

Layer L1 (logic, gpui-free). See `docs/plans/2026-11-18-rust-tree.md` section 4.6.

## Owns

The declarative settings model and everything the dialog decides without a window: the schema
(pages, sections, rows, controls, buttons), the built-in catalog of pages, setting values and the
ports that read, write and follow them, record lists and their port, shortcut capture (the
physical modifier keys, the capture rule, refusals), and `SettingsState`, the pure state machine
of the dialog (search, selection, drafts, validation, optimistic writes, prompts, rail moves).

## Public interface

- State machine: `SettingsState`, `Effect`, `Prompt`, `RailMove`.
- Ports and values: `SettingsBackend`, `SettingsRead`, `SettingsWrite`, `SettingsFollow`,
  `Listener`, `Subscription`, `Snapshot`, `Change`, `Key`, `Value`, `Values`, `Choice`, `Error`.
- Schema: `Schema`, `default_schema`, `preset_overrides`, `Page`, `PageId`, `Block`, `Section`,
  `Heading`, `ListBlock`, `Row`, `Condition`, `Control`, `ControlWidth`, `ChoiceSource`,
  `NumberRange`, `Nudge`, `Validator`, `Issue`, `ButtonSpec`, `Action`, `Confirm`, and the record
  list and keybinding constants (`AGENT_RECORDS`, `KEYBINDING_OVERRIDES`, ...).
- Shortcuts: `Modifier` (with `Modifier::primary`), `Pressed`, `Capture`, `Refusal`,
  `ChordSource`, `Shortcut`, `ShortcutGroup`.
- Records: `RecordBackend`, `Records`, `Record`, `Line`, `Tone`, `FlagLook`, `RecordFlag`,
  `RecordAction`, `RecordCommand`.
- Feature `fake`: `FakeBackend`, an in-memory backend for tests and the `settingsview` gallery.

## Depends on

- `chord`: the chord type, its parser and label, and the primary modifier.
- External: `thiserror`. It never depends on gpui.
