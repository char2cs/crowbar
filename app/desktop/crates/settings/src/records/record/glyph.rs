use crate::records::record::Record;
use crate::records::tone::Tone;

impl Record {
    #[must_use]
    pub fn icon(
        mut self,
        stem: impl Into<String>,
    ) -> Self {
        self.icon = Some(stem.into());
        self
    }

    #[must_use]
    pub fn artwork(
        mut self,
        markup: impl Into<String>,
    ) -> Self {
        self.artwork = Some(markup.into());
        self
    }

    #[must_use]
    pub fn status(
        mut self,
        tone: Tone,
    ) -> Self {
        self.status = Some(tone);
        self
    }

    #[must_use]
    pub fn icon_stem(&self) -> Option<&str> {
        self.icon.as_deref()
    }

    #[must_use]
    pub fn artwork_svg(&self) -> Option<&str> {
        self.artwork.as_deref()
    }

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
