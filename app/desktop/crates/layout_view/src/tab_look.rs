use gpui::SharedString;
use ui::IconName;

/// The label and glyph of a pane's tab in a strip.
#[derive(Debug, Clone, PartialEq)]
pub struct TabLook {
    /// The tab's text.
    pub label: SharedString,
    /// The tab's glyph.
    pub icon: IconName,
}

impl TabLook {
    /// A tab with only a name and the generic file glyph.
    pub fn plain(label: impl Into<SharedString>) -> Self {
        Self {
            label: label.into(),
            icon: IconName::FileText,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_plain_look_has_the_file_glyph() {
        let look = TabLook::plain("core.x");
        assert_eq!(look.label.as_ref(), "core.x", "label");
        assert_eq!(look.icon, IconName::FileText, "glyph");
    }
}
