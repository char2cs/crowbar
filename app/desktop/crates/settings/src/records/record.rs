//! One entry of a [`crate::ListBlock`].

use crate::records::line::Line;
use crate::records::record_action::RecordAction;
use crate::records::record_flag::RecordFlag;
use crate::records::tone::Tone;

mod controls;
mod glyph;

/// A record the backend lists: a title, lines of detail, optional status, badge, switches and one action.
#[derive(Debug, Clone, PartialEq)]
pub struct Record {
    pub(crate) id: String,
    pub(crate) title: String,
    pub(crate) icon: Option<String>,
    pub(crate) artwork: Option<String>,
    pub(crate) status: Option<Tone>,
    pub(crate) lines: Vec<Line>,
    pub(crate) badge: Option<Line>,
    pub(crate) flags: Vec<RecordFlag>,
    pub(crate) action: Option<RecordAction>,
    pub(crate) movable: bool,
}

impl Record {
    /// A record `id` titled `title`.
    pub fn new(
        id: impl Into<String>,
        title: impl Into<String>,
    ) -> Self {
        Self {
            id: id.into(),
            title: title.into(),
            icon: None,
            artwork: None,
            status: None,
            lines: Vec::new(),
            badge: None,
            flags: Vec::new(),
            action: None,
            movable: false,
        }
    }

    /// Appends a detail line.
    #[must_use]
    pub fn line(
        mut self,
        line: Line,
    ) -> Self {
        self.lines.push(line);
        self
    }

    /// Shows a badge after the title.
    #[must_use]
    pub fn badge(
        mut self,
        badge: Line,
    ) -> Self {
        self.badge = Some(badge);
        self
    }

    /// The record id.
    #[must_use]
    pub fn id(&self) -> &str {
        &self.id
    }

    /// The title text.
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    /// Detail lines under the title, in order.
    #[must_use]
    pub fn lines(&self) -> &[Line] {
        &self.lines
    }

    /// A badge shown after the title, if any.
    #[must_use]
    pub fn badge_line(&self) -> Option<&Line> {
        self.badge.as_ref()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detail_lines_append_in_order_and_the_badge_follows_the_title() {
        let record = Record::new("claude", "Claude Code")
            .line(Line::plain("v2.1"))
            .line(Line::plain("~/.claude"))
            .badge(Line::plain("OK").toned(Tone::Success));
        assert_eq!((record.id(), record.title()), ("claude", "Claude Code"));
        let texts: Vec<&str> = record.lines().iter().map(Line::text).collect();
        assert_eq!(texts, ["v2.1", "~/.claude"]);
        assert_eq!(record.badge_line().map(Line::tone), Some(Tone::Success));
    }
}
