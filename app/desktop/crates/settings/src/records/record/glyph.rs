use crate::records::record::Record;
use crate::records::tone::Tone;

impl Record {
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

    /// File stem of the Phosphor icon drawn before the title, if any.
    #[must_use]
    pub fn icon_stem(&self) -> Option<&str> {
        self.icon.as_deref()
    }

    /// SVG markup drawn before the title in place of a Phosphor icon, if any.
    #[must_use]
    pub fn artwork_svg(&self) -> Option<&str> {
        self.artwork.as_deref()
    }

    /// The tone of the status dot on the icon, if any.
    #[must_use]
    pub fn status_tone(&self) -> Option<Tone> {
        self.status
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_mark_before_the_title_is_drawn_only_when_given() {
        let bare = Record::new("claude", "Claude Code");
        assert_eq!(
            (bare.icon_stem(), bare.artwork_svg(), bare.status_tone()),
            (None, None, None)
        );
        let marked = bare.icon("robot").artwork("<svg/>").status(Tone::Success);
        assert_eq!(marked.icon_stem(), Some("robot"));
        assert_eq!(marked.artwork_svg(), Some("<svg/>"));
        assert_eq!(marked.status_tone(), Some(Tone::Success));
    }
}
