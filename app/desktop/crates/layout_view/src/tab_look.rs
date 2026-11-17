use gpui::SharedString;
use ui::IconName;

/// The label and glyph of a pane's tab in a strip.
#[derive(Debug, Clone, PartialEq)]
pub struct TabLook {
    /// The tab's text.
    pub label: SharedString,
    /// The tab's glyph.
    pub icon: IconName,
    /// The file the tab shows, relative to its workspace; the tab menu offers path actions
    /// only for a tab that has one.
    pub path: Option<String>,
    /// Whether the pane holds unsaved work.
    pub dirty: bool,
}

impl TabLook {
    /// A tab with only a name and the generic file glyph.
    pub fn plain(label: impl Into<SharedString>) -> Self {
        Self {
            label: label.into(),
            icon: IconName::FileText,
            path: None,
            dirty: false,
        }
    }

    /// The same tab showing the file at `path`, relative to its workspace.
    #[must_use]
    pub fn at_path(
        mut self,
        path: impl Into<String>,
    ) -> Self {
        self.path = Some(path.into());
        self
    }

    /// The same tab marked as holding unsaved work.
    #[must_use]
    pub fn unsaved(mut self) -> Self {
        self.dirty = true;
        self
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
        assert_eq!(
            (look.path, look.dirty),
            (None, false),
            "no file, nothing unsaved"
        );
    }

    #[test]
    fn a_look_can_name_its_file_and_mark_unsaved_work() {
        let look = TabLook::plain("main.rs").at_path("src/main.rs").unsaved();
        assert_eq!(
            (look.path.as_deref(), look.dirty),
            (Some("src/main.rs"), true),
            "file and mark"
        );
    }
}
