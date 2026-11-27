#[cfg(feature = "fake")]
mod fake_backend;
mod flag_look;
mod line;
mod record;
mod record_action;
mod record_backend;
mod record_command;
mod record_flag;
mod tone;

#[cfg(feature = "fake")]
pub use fake_backend::FakeBackend;
pub use flag_look::FlagLook;
pub use line::Line;
pub use record::Record;
pub use record_action::RecordAction;
pub use record_backend::RecordBackend;
pub use record_command::RecordCommand;
pub use record_flag::RecordFlag;
pub use tone::Tone;

#[derive(Debug, Clone, PartialEq)]
pub enum Records {
    Loading,
    Failed,
    Ready(Vec<Record>),
}
