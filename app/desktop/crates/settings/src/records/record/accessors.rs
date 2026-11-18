//! The read side of a [`Record`]: what the settings view draws from it.

use crate::records::line::Line;
use crate::records::record::Record;
use crate::records::record_action::RecordAction;
use crate::records::record_flag::RecordFlag;
use crate::records::tone::Tone;

impl Record {
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

    /// The tone of the status dot on the icon, if any.
    #[must_use]
    pub fn status_tone(&self) -> Option<Tone> {
        self.status
    }

    /// Whether the user can drag the record to reorder the list.
    #[must_use]
    pub fn is_movable(&self) -> bool {
        self.movable
    }

    /// Detail lines under the title, in order.
    #[must_use]
    pub fn lines(&self) -> &[Line] {
        &self.lines
    }

    /// File stem of the Phosphor icon drawn before the title, if any.
    #[must_use]
    pub fn icon_stem(&self) -> Option<&str> {
        self.icon.as_deref()
    }

    /// Switches, each shown in the column of the same position.
    #[must_use]
    pub fn flags(&self) -> &[RecordFlag] {
        &self.flags
    }

    /// A badge shown after the title, if any.
    #[must_use]
    pub fn badge_line(&self) -> Option<&Line> {
        self.badge.as_ref()
    }

    /// SVG markup drawn before the title in place of a Phosphor icon, if any.
    #[must_use]
    pub fn artwork_svg(&self) -> Option<&str> {
        self.artwork.as_deref()
    }

    /// The one action the record offers, if any.
    #[must_use]
    pub fn offered_action(&self) -> Option<&RecordAction> {
        self.action.as_ref()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_bare_record_reads_back_only_its_id_and_title() {
        let record = Record::new("claude", "Claude Code");
        assert_eq!((record.id(), record.title()), ("claude", "Claude Code"));
        assert_eq!((record.icon_stem(), record.artwork_svg()), (None, None));
        assert_eq!(record.status_tone(), None);
        assert!(record.lines().is_empty() && record.flags().is_empty());
        assert!(record.badge_line().is_none() && record.offered_action().is_none());
        assert!(!record.is_movable());
    }
}
