use gpui::SharedString;
use ui::{IconName, TabPreview};

use crate::tabbar::tab_access::TabAccess;
use crate::tabbar::tab_keep::TabKeep;

/// The label and glyph of a pane's tab in a strip.
#[derive(Debug, Clone, PartialEq)]
pub struct TabLook {
    /// The tab's text.
    pub label: SharedString,
    /// The tab's glyph.
    pub icon: IconName,
    /// A chat provider's SVG artwork drawn in place of `icon`, once it is known.
    pub artwork: Option<SharedString>,
    /// The file the tab shows, relative to its workspace; the tab menu offers path actions
    /// only for a tab that has one.
    pub path: Option<String>,
    /// Whether the pane holds unsaved work.
    pub dirty: bool,
    /// Whether the tab menu offers to rename the tab.
    pub renamable: bool,
    /// Whether the tab is a preview or kept.
    pub keep: TabKeep,
    /// Whether the tab's file can be written.
    pub access: TabAccess,
}

impl TabLook {
    /// A tab with only a name and the generic file glyph.
    pub fn plain(label: impl Into<SharedString>) -> Self {
        Self {
            label: label.into(),
            icon: IconName::FileText,
            artwork: None,
            path: None,
            dirty: false,
            renamable: false,
            keep: TabKeep::Kept,
            access: TabAccess::Writable,
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

    /// The same tab drawing the SVG `artwork` in place of its glyph.
    #[must_use]
    pub fn drawn(
        mut self,
        artwork: impl Into<SharedString>,
    ) -> Self {
        self.artwork = Some(artwork.into());
        self
    }

    /// The same tab, which its menu offers to rename.
    #[must_use]
    pub fn renamable(mut self) -> Self {
        self.renamable = true;
        self
    }

    /// The same tab marked as a preview.
    #[must_use]
    pub fn previewed(mut self) -> Self {
        self.keep = TabKeep::Preview;
        self
    }

    /// The same tab, whose file's branch is locked.
    #[must_use]
    pub fn read_only(mut self) -> Self {
        self.access = TabAccess::ReadOnly;
        self
    }

    /// The same tab marked as holding unsaved work.
    #[must_use]
    pub fn unsaved(mut self) -> Self {
        self.dirty = true;
        self
    }

    /// The preview lifted under the pointer while the tab is dragged.
    #[must_use]
    pub fn drag_preview(&self) -> TabPreview {
        let preview = TabPreview::new(self.label.clone());
        if !self.dirty {
            return preview;
        }
        preview.unsaved()
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
            (look.path.as_deref(), look.dirty, look.renamable),
            (Some("src/main.rs"), true, false),
            "file and mark"
        );
        assert!(TabLook::plain("zsh").renamable().renamable, "renamable");
    }
}
