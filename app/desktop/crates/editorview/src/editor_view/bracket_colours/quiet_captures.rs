use gpui::HighlightStyle;
use gpui_kit::base::input::HighlightStyleResolver;

const MARK: f32 = 1.0;

pub(super) struct QuietCaptures {
    names: &'static [&'static str],
}

impl QuietCaptures {
    pub(super) const STRINGS_AND_COMMENTS: Self = Self {
        names: &["string", "comment", "text.literal", "link_uri"],
    };

    pub(super) fn is_quiet(style: &HighlightStyle) -> bool {
        style.fade_out == Some(MARK)
    }
}

impl HighlightStyleResolver for QuietCaptures {
    fn style(
        &self,
        name: &str,
    ) -> Option<HighlightStyle> {
        let quiet = self.names.iter().any(|quiet| name.starts_with(quiet));
        quiet.then(|| HighlightStyle {
            fade_out: Some(MARK),
            ..HighlightStyle::default()
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_string_or_comment_capture_and_its_dotted_kinds_are_quiet() {
        let quiet = QuietCaptures::STRINGS_AND_COMMENTS;
        let styled = |name| {
            quiet
                .style(name)
                .is_some_and(|style| QuietCaptures::is_quiet(&style))
        };
        assert!(
            ["string", "string.special", "comment.doc", "link_uri"]
                .into_iter()
                .all(styled)
        );
        assert!(!["punctuation.bracket", "variable"].into_iter().any(styled));
    }
}
