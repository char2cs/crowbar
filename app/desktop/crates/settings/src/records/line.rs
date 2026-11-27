use crate::records::tone::Tone;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Line {
    pub(crate) text: String,
    pub(crate) mono: bool,
    pub(crate) tone: Tone,
}

impl Line {
    #[must_use]
    pub fn tone(&self) -> Tone {
        self.tone
    }

    #[must_use]
    pub fn text(&self) -> &str {
        &self.text
    }

    #[must_use]
    pub fn is_mono(&self) -> bool {
        self.mono
    }

    pub fn plain(text: impl Into<String>) -> Self {
        Self {
            text: text.into(),
            mono: false,
            tone: Tone::Neutral,
        }
    }

    #[must_use]
    pub fn mono(mut self) -> Self {
        self.mono = true;
        self
    }

    #[must_use]
    pub fn toned(
        mut self,
        tone: Tone,
    ) -> Self {
        self.tone = tone;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_change_family_and_tone() {
        let line = Line::plain("homelab:3737").mono().toned(Tone::Error);
        assert_eq!(
            (line.text.as_str(), line.mono, line.tone),
            ("homelab:3737", true, Tone::Error)
        );
        assert_eq!(Line::plain("x").tone, Tone::Neutral);
    }
}
