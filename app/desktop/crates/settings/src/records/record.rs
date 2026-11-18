//! One entry of a [`crate::ListBlock`].

use crate::records::line::Line;
use crate::records::record_action::RecordAction;
use crate::records::record_flag::RecordFlag;
use crate::records::tone::Tone;

mod accessors;

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

    /// Draws the Phosphor icon with this file stem before the title.
    #[must_use]
    pub fn icon(
        mut self,
        stem: impl Into<String>,
    ) -> Self {
        self.icon = Some(stem.into());
        self
    }

    /// Draws the SVG `markup` before the title, in place of a Phosphor icon. The caller vets the
    /// markup: it is drawn as given.
    #[must_use]
    pub fn artwork(
        mut self,
        markup: impl Into<String>,
    ) -> Self {
        self.artwork = Some(markup.into());
        self
    }

    /// Shows a status dot of `tone` on the icon.
    #[must_use]
    pub fn status(
        mut self,
        tone: Tone,
    ) -> Self {
        self.status = Some(tone);
        self
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

    /// Appends a switch.
    #[must_use]
    pub fn flag(
        mut self,
        flag: RecordFlag,
    ) -> Self {
        self.flags.push(flag);
        self
    }

    /// Offers `action` on the record.
    #[must_use]
    pub fn action(
        mut self,
        action: RecordAction,
    ) -> Self {
        self.action = Some(action);
        self
    }

    /// Lets the user drag the record to reorder the list.
    #[must_use]
    pub fn movable(mut self) -> Self {
        self.movable = true;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_compose_a_full_record() {
        let record = Record::new("claude", "Claude Code")
            .icon("robot")
            .artwork("<svg/>")
            .status(Tone::Success)
            .line(Line::plain("v2.1"))
            .badge(Line::plain("OK").toned(Tone::Success))
            .flag(RecordFlag::new("enabled", "Enable").on())
            .action(RecordAction::new("kill", "skull", "Kill"))
            .movable();
        assert_eq!(record.id(), "claude");
        assert_eq!(record.icon.as_deref(), Some("robot"));
        assert_eq!(record.artwork.as_deref(), Some("<svg/>"));
        assert_eq!(record.status, Some(Tone::Success));
        assert_eq!((record.lines.len(), record.flags.len()), (1, 1));
        assert!(record.badge.is_some() && record.action.is_some() && record.movable);
    }
}
