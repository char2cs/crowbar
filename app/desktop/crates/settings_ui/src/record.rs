//! One entry of a [`crate::ListBlock`].

use crate::line::Line;
use crate::record_action::RecordAction;
use crate::record_flag::RecordFlag;
use crate::tone::Tone;

/// A record the backend lists: a title, lines of detail, optional status, badge, switches and one action.
#[derive(Debug, Clone, PartialEq)]
pub struct Record {
    pub(crate) id: String,
    pub(crate) title: String,
    pub(crate) icon: Option<String>,
    pub(crate) status: Option<Tone>,
    pub(crate) lines: Vec<Line>,
    pub(crate) badge: Option<Line>,
    pub(crate) flags: Vec<RecordFlag>,
    pub(crate) action: Option<RecordAction>,
    pub(crate) movable: bool,
}

impl Record {
    /// A record `id` titled `title`.
    pub fn new(id: impl Into<String>, title: impl Into<String>) -> Self {
        Self {
            id: id.into(),
            title: title.into(),
            icon: None,
            status: None,
            lines: Vec::new(),
            badge: None,
            flags: Vec::new(),
            action: None,
            movable: false,
        }
    }

    /// Draws the Phosphor icon with this file stem before the title.
    #[must_use]
    pub fn icon(mut self, stem: impl Into<String>) -> Self {
        self.icon = Some(stem.into());
        self
    }

    /// Shows a status dot of `tone` on the icon.
    #[must_use]
    pub fn status(mut self, tone: Tone) -> Self {
        self.status = Some(tone);
        self
    }

    /// Appends a detail line.
    #[must_use]
    pub fn line(mut self, line: Line) -> Self {
        self.lines.push(line);
        self
    }

    /// Shows a badge after the title.
    #[must_use]
    pub fn badge(mut self, badge: Line) -> Self {
        self.badge = Some(badge);
        self
    }

    /// Appends a switch.
    #[must_use]
    pub fn flag(mut self, flag: RecordFlag) -> Self {
        self.flags.push(flag);
        self
    }

    /// Offers `action` on the record.
    #[must_use]
    pub fn action(mut self, action: RecordAction) -> Self {
        self.action = Some(action);
        self
    }

    /// Lets the user drag the record to reorder the list.
    #[must_use]
    pub fn movable(mut self) -> Self {
        self.movable = true;
        self
    }

    /// The record id.
    pub fn id(&self) -> &str {
        &self.id
    }
}

#[cfg(test)]
#[path = "record_tests.rs"]
mod tests;
