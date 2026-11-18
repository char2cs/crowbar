//! The settings model of the Crowbar shell without a window.
//!
//! The schema ([`Schema`]) says what the settings dialog shows; [`SettingsState`] is the pure state
//! machine (search, selection, drafts, validation, optimistic writes, shortcut capture);
//! [`SettingsBackend`] and [`RecordBackend`] are the only doors to the outside world. The
//! `settingsview` crate draws it with gpui.

mod action;
mod block;
mod button_spec;
mod capture;
mod catalog;
mod change;
mod choice;
mod choice_source;
mod chord_capture;
mod chord_source;
mod condition;
mod confirm;
mod control;
mod control_text;
mod control_width;
mod document;
mod document_reads;
mod effect;
mod error;
#[cfg(feature = "fake")]
mod fake_backend;
#[cfg(feature = "fake")]
mod fake_records;
#[cfg(feature = "fake")]
mod fake_settings;
#[cfg(feature = "fake")]
mod fake_state;
mod filter;
mod flag_look;
mod heading;
mod issue;
mod key;
mod keymap;
mod line;
mod list_block;
mod modifier;
mod navigation;
mod nudge;
mod number_range;
mod page;
mod page_id;
mod pressed;
mod prompt;
mod rail_move;
mod record;
mod record_action;
mod record_backend;
mod record_command;
mod record_flag;
mod records;
mod refusal;
mod row;
mod schema;
mod scope;
mod section;
mod settings_backend;
mod settings_follow;
mod settings_read;
mod settings_state;
mod settings_write;
mod shortcut;
mod shortcut_group;
mod snapshot;
mod subscription;
mod test_support;
mod tone;
mod validator;
mod value;
mod values;
mod visibility;

pub use action::Action;
pub use block::Block;
pub use button_spec::ButtonSpec;
pub use capture::Capture;
pub use catalog::{
    AGENT_RECORDS, DESCRIPTOR_RECORDS, HOST_RECORDS, KEYBINDING_OVERRIDES, KEYBINDING_PRESET,
    SESSION_RECORDS, default_schema, preset_overrides,
};
pub use change::Change;
pub use choice::Choice;
pub use choice_source::ChoiceSource;
pub use chord_source::ChordSource;
pub use condition::Condition;
pub use confirm::Confirm;
pub use control::Control;
pub use control_width::ControlWidth;
pub use effect::Effect;
pub use error::Error;
#[cfg(feature = "fake")]
pub use fake_backend::FakeBackend;
pub use flag_look::FlagLook;
pub use heading::Heading;
pub use issue::Issue;
pub use key::Key;
pub use line::Line;
pub use list_block::ListBlock;
pub use modifier::Modifier;
pub use nudge::Nudge;
pub use number_range::NumberRange;
pub use page::Page;
pub use page_id::PageId;
pub use pressed::Pressed;
pub use prompt::Prompt;
pub use rail_move::RailMove;
pub use record::Record;
pub use record_action::RecordAction;
pub use record_backend::RecordBackend;
pub use record_command::RecordCommand;
pub use record_flag::RecordFlag;
pub use records::Records;
pub use refusal::Refusal;
pub use row::Row;
pub use schema::Schema;
pub use section::Section;
pub use settings_backend::SettingsBackend;
pub use settings_follow::{Listener, SettingsFollow};
pub use settings_read::SettingsRead;
pub use settings_state::SettingsState;
pub use settings_write::SettingsWrite;
pub use shortcut::Shortcut;
pub use shortcut_group::ShortcutGroup;
pub use snapshot::Snapshot;
pub use subscription::Subscription;
pub use tone::Tone;
pub use validator::Validator;
pub use value::Value;
pub use values::Values;
