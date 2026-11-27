//! The settings model of the Crowbar shell without a window.
//!
//! The schema ([`Schema`]) says what the settings dialog shows; [`State`] is the pure state
//! machine (search, selection, drafts, validation, optimistic writes, shortcut capture);
//! [`Backend`] and [`RecordBackend`] are the only doors to the outside world. The
//! `settingsview` crate draws it with gpui.

mod catalog;
mod keys;
mod records;
mod schema;
mod state;
mod test_support;
mod values;

pub use catalog::{
    AGENT_RECORDS, DESCRIPTOR_RECORDS, DEVICE_ADDRESS, DEVICE_CONNECTIONS, DEVICE_LISTENING,
    DEVICE_RECORDS, HOST_RECORDS, KEYBINDING_OVERRIDES, KEYBINDING_PRESET, SESSION_RECORDS,
    default_schema, preset_overrides,
};
pub use keys::Capture;
pub use keys::ChordSource;
pub use keys::{ModifierKey, Pressed, Refusal, Shortcut, ShortcutGroup};
#[cfg(feature = "fake")]
pub use records::FakeBackend;
pub use records::{
    FlagLook, Line, Record, RecordAction, RecordBackend, RecordCommand, RecordFlag, Records, Tone,
};
pub use schema::{Action, Block, ButtonFace, ButtonSpec};
pub use schema::{ChoiceSource, Confirm, Control, ControlWidth};
pub use schema::{
    Heading, Issue, LabelStyle, ListBlock, Nudge, NumberRange, Page, PageId, Row, Schema, Section,
    Validator,
};
pub use state::Effect;
pub use state::{Prompt, RailMove, State};
pub use values::{
    Backend, Follow, Key, Listener, Read, Snapshot, Subscription, Value, Values, Write,
};
pub use values::{Change, Choice, Error};
