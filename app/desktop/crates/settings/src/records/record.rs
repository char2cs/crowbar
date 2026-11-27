use crate::records::line::Line;
use crate::records::record_action::RecordAction;
use crate::records::record_flag::RecordFlag;
use crate::records::tone::Tone;

mod controls;
mod glyph;

#[derive(Debug, Clone, PartialEq)]
pub struct Record {
    pub(crate) id: String,
    pub(crate) title: String,
    pub(crate) icon: Option<String>,
    pub(crate) artwork: Option<String>,
    pub(crate) status: Option<Tone>,
    pub(crate) lines: Vec<Line>,
    pub(crate) aside: Option<Line>,
    pub(crate) badge: Option<Line>,
    pub(crate) flags: Vec<RecordFlag>,
    pub(crate) action: Option<RecordAction>,
    pub(crate) movable: bool,
}

impl Record {
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
            aside: None,
            badge: None,
            flags: Vec::new(),
            action: None,
            movable: false,
        }
    }

    #[must_use]
    pub fn line(
        mut self,
        line: Line,
    ) -> Self {
        self.lines.push(line);
        self
    }

    #[must_use]
    pub fn aside(
        mut self,
        aside: Line,
    ) -> Self {
        self.aside = Some(aside);
        self
    }

    #[must_use]
    pub fn aside_line(&self) -> Option<&Line> {
        self.aside.as_ref()
    }

    #[must_use]
    pub fn badge(
        mut self,
        badge: Line,
    ) -> Self {
        self.badge = Some(badge);
        self
    }

    #[must_use]
    pub fn id(&self) -> &str {
        &self.id
    }

    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    #[must_use]
    pub fn lines(&self) -> &[Line] {
        &self.lines
    }

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
            .aside(Line::plain("shipped"))
            .badge(Line::plain("OK").toned(Tone::Success));
        assert_eq!((record.id(), record.title()), ("claude", "Claude Code"));
        let texts: Vec<&str> = record.lines().iter().map(Line::text).collect();
        assert_eq!(texts, ["v2.1", "~/.claude"]);
        assert_eq!(record.badge_line().map(Line::tone), Some(Tone::Success));
        assert_eq!(record.aside_line().map(Line::text), Some("shipped"));
    }
}
