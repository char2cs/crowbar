//! The settings model of the Crowbar shell without a window.
//!
//! The schema ([`Schema`]) says what the settings dialog shows; [`SettingsState`] is the pure state
//! machine (search, selection, drafts, validation, optimistic writes, shortcut capture);
//! [`SettingsBackend`] and [`RecordBackend`] are the only doors to the outside world. The
//! `settingsview` crate draws it with gpui.

mod catalog;
mod keys;
mod records;
mod schema;
mod settings_state;
mod test_support;
mod values;

pub use catalog::{
    AGENT_RECORDS, DESCRIPTOR_RECORDS, HOST_RECORDS, KEYBINDING_OVERRIDES, KEYBINDING_PRESET,
    SESSION_RECORDS, default_schema, preset_overrides,
};
pub use keys::Capture;
pub use keys::ChordSource;
pub use keys::{Modifier, Pressed, Refusal, Shortcut, ShortcutGroup};
#[cfg(feature = "fake")]
pub use records::FakeBackend;
pub use records::{
    FlagLook, Line, Record, RecordAction, RecordBackend, RecordCommand, RecordFlag, Records, Tone,
};
pub use schema::{Action, Block, ButtonSpec};
pub use schema::{ChoiceSource, Condition, Confirm, Control, ControlWidth};
pub use schema::{
    Heading, Issue, ListBlock, Nudge, NumberRange, Page, PageId, Row, Schema, Section, Validator,
};
pub use settings_state::Effect;
pub use settings_state::{Prompt, RailMove, SettingsState};
pub use values::{Change, Choice, Error};
pub use values::{
    Key, Listener, SettingsBackend, SettingsFollow, SettingsRead, SettingsWrite, Snapshot,
    Subscription, Value, Values,
};
