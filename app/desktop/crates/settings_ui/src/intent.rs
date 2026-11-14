//! What the user asked of the modal.

use ui::TextInputEvent;

use crate::button_spec::ButtonSpec;
use crate::nudge::Nudge;
use crate::page_id::PageId;
use crate::pressed::Pressed;
use crate::rail_move::RailMove;
use crate::record_action::RecordAction;
use crate::row::Row;

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
