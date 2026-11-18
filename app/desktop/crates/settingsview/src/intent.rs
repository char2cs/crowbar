//! What the user asked of the modal.

mod intent_runner;
mod intent_sink;

pub(crate) use intent_runner::IntentRunner;
pub(crate) use intent_sink::IntentSink;

use ui::TextInputEvent;

use settings::{ButtonSpec, Nudge, PageId, Pressed, RailMove, RecordAction, Row};

#[derive(Debug, Clone)]
pub(crate) enum Intent {
    Toggle(Row),
    Choose(Row, usize),
    Edit(Row, TextInputEvent),
    Commit(Row),
    Nudge(Row, Nudge),
    Reset(Row),
    Button(ButtonSpec),
    Key(Pressed),
    Select(PageId),
    Move(RailMove),
    Search(TextInputEvent),
    Capture(String),
    ClearChord(String),
    Flag {
        source: String,
        record: String,
        flag: String,
        on: bool,
    },
    Reorder {
        source: String,
        record: String,
        to: usize,
    },
    RunRecord {
        source: String,
        record: String,
        action: RecordAction,
    },
    Retry(String),
    Accept,
    Dismiss,
    Close,
}
