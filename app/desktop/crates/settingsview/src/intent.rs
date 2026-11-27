mod runner;
mod sink;

pub(crate) use runner::Runner;
pub(crate) use sink::Sink;

use ui::TextInputEvent;

use crate::lift::Lift;
use settings::{ButtonSpec, Nudge, PageId, Pressed, RailMove, RecordAction, Row};

#[derive(Debug, Clone)]
pub(crate) enum Intent {
    Toggle(Row),
    Activate(Row),
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
    Lift(Lift),
    Land,
    Retry(String),
    Accept,
    Dismiss,
    Close,
}
