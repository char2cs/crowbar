use gpui::SharedString;
use ui::{IconName, TabPreview};

use crate::tabbar::look::tab_access::TabAccess;
use crate::tabbar::look::tab_keep::TabKeep;

#[derive(Debug, Clone, PartialEq)]
pub struct TabLook {
    pub label: SharedString,
    pub icon: IconName,
    pub artwork: Option<SharedString>,
    pub path: Option<String>,
    pub dirty: bool,
    pub renamable: bool,
    pub keep: TabKeep,
    pub access: TabAccess,
}

impl TabLook {
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

    #[must_use]
    pub fn at_path(
        mut self,
        path: impl Into<String>,
    ) -> Self {
        self.path = Some(path.into());
        self
    }

    #[must_use]
    pub fn drawn(
        mut self,
        artwork: impl Into<SharedString>,
    ) -> Self {
        self.artwork = Some(artwork.into());
        self
    }

    #[must_use]
    pub fn renamable(mut self) -> Self {
        self.renamable = true;
        self
    }

    #[must_use]
    pub fn previewed(mut self) -> Self {
        self.keep = TabKeep::Preview;
        self
    }

    #[must_use]
    pub fn read_only(mut self) -> Self {
        self.access = TabAccess::ReadOnly;
        self
    }

    #[must_use]
    pub fn unsaved(mut self) -> Self {
        self.dirty = true;
        self
    }

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
