//! The dialog: a tree of the host's folders, one of which is chosen.

use std::collections::BTreeSet;
use std::rc::Rc;
use std::sync::Arc;

use gpui::{Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Window, div};
use shell_data::{BrowseEntry, FolderBrowse, FolderListing, ROOT_PATH};
use ui::{Dialog, DialogSize, Surface, Theme};

use crate::folder_reader::FolderReader;
use crate::folder_requests::FolderRequests;
use crate::folder_source::FolderSource;
use crate::muted::muted;
use crate::picker_event::PickerEvent;
use crate::row_look::{GAP, TITLE};

/// The remote folder picker; it emits a [`PickerEvent`] when a folder is chosen or it is dismissed.
pub struct RemoteFolderPicker {
    pub(crate) source: Rc<dyn FolderSource>,
    pub(crate) expanded: BTreeSet<String>,
    pub(crate) selected: Option<BrowseEntry>,
}

impl std::fmt::Debug for RemoteFolderPicker {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("RemoteFolderPicker")
            .field("selected", &self.selected)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<PickerEvent> for RemoteFolderPicker {}

impl FolderReader for Arc<FolderBrowse> {
    fn host_name(&self) -> String {
        FolderBrowse::host_name(self)
    }

    fn can_ask(&self) -> bool {
        FolderBrowse::can_ask(self)
    }

    fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing> {
        FolderBrowse::listing(self, path)
    }
}

impl FolderRequests for Arc<FolderBrowse> {
    fn request(
        &self,
        path: &str,
    ) {
        FolderBrowse::request(self, path);
    }

    fn retry(
        &self,
        path: &str,
    ) {
        FolderBrowse::retry(self, path);
    }
}

impl RemoteFolderPicker {
    /// A picker over the folders of `browse`'s host.
    pub fn new(browse: Arc<FolderBrowse>) -> Self {
        Self::over(Rc::new(browse))
    }

    pub(crate) fn over(source: Rc<dyn FolderSource>) -> Self {
        Self {
            source,
            expanded: BTreeSet::new(),
            selected: None,
        }
    }

    pub(crate) fn ask_for_what_is_shown(&self) {
        self.source.request(ROOT_PATH);
        self.expanded
            .iter()
            .for_each(|path| self.source.request(path));
    }
}

impl Render for RemoteFolderPicker {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        self.ask_for_what_is_shown();
        let palette = *Theme::of(cx).palette(Surface::Window);
        let host = self.source.host_name();
        let me = cx.entity();
        let body = div()
            .flex()
            .flex_col()
            .gap(GAP)
            .child(muted(&palette).child(format!("Browsing {host}'s filesystem")))
            .children(self.banner(&palette))
            .child(self.root(&palette, cx));
        Dialog::new("remote-folders")
            .title(TITLE)
            .size(DialogSize::Sm)
            .body(body)
            .footer(self.footer(cx))
            .on_close(move |_, cx| me.update(cx, |_, cx| Self::cancel(cx)))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::{Events, open, reachable_fake};

    #[gpui::test]
    fn rendering_asks_for_the_root_and_for_every_opened_folder(cx: &mut TestAppContext) {
        let (fake, events) = (reachable_fake(), Events::default());
        let (picker, cx) = open(cx, &fake, &events);
        cx.run_until_parked();
        picker.update(cx, |this, cx| {
            this.expanded.insert("/srv".to_owned());
            cx.notify();
        });
        cx.run_until_parked();
        let asked = fake.asked.borrow().clone();
        assert!(asked.contains(&"/".to_owned()), "the root is asked for");
        assert!(asked.contains(&"/srv".to_owned()), "the opened folder too");
    }
}
