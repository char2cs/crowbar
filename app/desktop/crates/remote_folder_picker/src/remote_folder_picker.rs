//! The dialog: a tree of the host's folders, one of which is chosen.

use std::collections::BTreeSet;
use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    AnyElement, Context, Div, EventEmitter, InteractiveElement, IntoElement, ParentElement, Render,
    SharedString, StatefulInteractiveElement, Styled, Window, div, uniform_list,
};
use shell_data::{BrowseEntry, FolderBrowse, FolderListing, ROOT_PATH};
use ui::{Button, ButtonSize, ButtonVariant, Dialog, DialogSize, Palette, Pose, Surface, Theme};

use crate::folder_row::FolderRow;
use crate::folder_rows::{children_of, rows};
use crate::folder_source::FolderSource;
use crate::picker_event::PickerEvent;
use crate::row_look::{
    BANNER_TINT, EMPTY_ROOT, GAP, LOADING_ROOT, ROW_HEIGHT, TEXT, TITLE, TREE_HEIGHT,
};
use crate::shown::Shown;
use crate::tree_row::{Toggle, tree_row};

/// The remote folder picker; it emits a [`PickerEvent`] when a folder is chosen or it is dismissed.
pub struct RemoteFolderPicker {
    source: Rc<dyn FolderSource>,
    expanded: BTreeSet<String>,
    selected: Option<BrowseEntry>,
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

impl FolderSource for Arc<FolderBrowse> {
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

fn muted(palette: &Palette) -> Div {
    div().text_size(TEXT).text_color(palette.muted_foreground())
}

fn centred(
    palette: &Palette,
    text: impl Into<SharedString>,
) -> Div {
    muted(palette)
        .h(TREE_HEIGHT)
        .flex()
        .items_center()
        .justify_center()
        .child(text.into())
}

fn highlighted(
    row: Div,
    palette: &Palette,
    chosen: Pose,
) -> Div {
    match chosen {
        Pose::On => row.bg(palette.accent()),
        Pose::Off => row,
    }
}

fn toggler(
    view: &gpui::Entity<RemoteFolderPicker>,
    path: &str,
) -> Toggle {
    let (view, path) = (view.clone(), path.to_owned());
    Box::new(move |_, cx| {
        cx.stop_propagation();
        view.update(cx, |this, cx| this.toggle(&path, cx));
    })
}

fn inert() -> Toggle {
    Box::new(|_, _| {})
}

impl RemoteFolderPicker {
    /// A picker over the folders of `browse`'s host.
    pub fn new(browse: Arc<FolderBrowse>) -> Self {
        Self::over(Rc::new(browse))
    }

    fn over(source: Rc<dyn FolderSource>) -> Self {
        Self {
            source,
            expanded: BTreeSet::new(),
            selected: None,
        }
    }

    fn choose(
        &mut self,
        entry: &BrowseEntry,
        cx: &mut Context<'_, Self>,
    ) {
        self.selected = Some(entry.clone());
        self.expanded.insert(entry.path.clone());
        cx.notify();
    }

    fn toggle(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.expanded.remove(path) {
            self.expanded.insert(path.to_owned());
        }
        cx.notify();
    }

    fn confirm(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chosen) = self.selected.as_ref().filter(|_| self.source.can_ask()) else {
            return;
        };
        cx.emit(PickerEvent::Chosen(chosen.path.clone()));
    }

    fn cancel(cx: &mut Context<'_, Self>) {
        cx.emit(PickerEvent::Cancelled);
    }

    fn ask_for_what_is_shown(&self) {
        self.source.request(ROOT_PATH);
        self.expanded
            .iter()
            .for_each(|path| self.source.request(path));
    }

    fn row_element(
        &self,
        (index, row): (usize, &FolderRow),
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let host = self.source.host_name();
        let FolderRow::Entry { entry, .. } = row else {
            return tree_row(index, row, (&host, palette), inert()).into_any_element();
        };
        let toggle = toggler(&cx.entity(), &entry.path);
        let picked = entry.clone();
        let chosen = self
            .selected
            .as_ref()
            .is_some_and(|held| held.path == entry.path);
        let styled = tree_row(index, row, (&host, palette), toggle);
        highlighted(styled, palette, Pose::from(chosen))
            .id(("folder-row", index))
            .on_click(cx.listener(move |this, _, _, cx| this.choose(&picked, cx)))
            .into_any_element()
    }

    fn tree(
        &self,
        entries: &[BrowseEntry],
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let shown = Shown {
            source: self.source.as_ref(),
            expanded: &self.expanded,
        };
        let all = rows(ROOT_PATH, entries, &shown, 1);
        let count = all.len();
        let palette = *palette;
        uniform_list(
            "remote-folders",
            count,
            cx.processor(move |this, range: std::ops::Range<usize>, _, cx| {
                range
                    .filter_map(|index| all.get(index).map(|row| (index, row)))
                    .map(|pair| this.row_element(pair, &palette, cx))
                    .collect::<Vec<_>>()
            }),
        )
        .h(TREE_HEIGHT)
        .into_any_element()
    }

    fn failure(
        &self,
        (reason, palette): (&str, &Palette),
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let host = self.source.host_name();
        let mut retry = Button::new("folders-retry", "Retry")
            .variant(ButtonVariant::Outline)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.retry_root(cx)));
        if !self.source.can_ask() {
            retry = retry.disabled();
        }
        centred(
            palette,
            format!("Couldn't list folders on {host}: {reason}"),
        )
        .flex_col()
        .gap(GAP)
        .child(retry)
        .into_any_element()
    }

    fn retry_root(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.source.retry(ROOT_PATH);
        cx.notify();
    }

    fn root(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let host = self.source.host_name();
        match self.source.listing(ROOT_PATH) {
            None if !self.source.can_ask() => {
                centred(palette, format!("Folders load once {host} answers.")).into_any_element()
            },
            None | Some(FolderListing::Loading) => {
                centred(palette, LOADING_ROOT).into_any_element()
            },
            Some(FolderListing::Failed(reason)) => self.failure((&reason, palette), cx),
            Some(FolderListing::Loaded(entries)) if children_of(ROOT_PATH, &entries).is_empty() => {
                centred(palette, EMPTY_ROOT).into_any_element()
            },
            Some(FolderListing::Loaded(entries)) => self.tree(&entries, palette, cx),
        }
    }

    fn footer(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let label = self.selected.as_ref().map_or_else(
            || "Select".to_owned(),
            |entry| format!("Select \"{}\"", entry.name),
        );
        let mut select = Button::new("folders-select", label)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|this, _, _, cx| this.confirm(cx)));
        if self.selected.is_none() || !self.source.can_ask() {
            select = select.disabled();
        }
        let cancel = Button::new("folders-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|_, _, _, cx| Self::cancel(cx)));
        div().flex().gap(GAP).child(cancel).child(select)
    }

    fn banner(
        &self,
        palette: &Palette,
    ) -> Option<Div> {
        if self.source.can_ask() {
            return None;
        }
        let host = self.source.host_name();
        let text = format!("{host} is not answering. Its folders load again once it does.");
        Some(
            div()
                .px(GAP * 2.0)
                .h(ROW_HEIGHT)
                .flex()
                .items_center()
                .bg(palette.warning().opacity(BANNER_TINT))
                .text_size(TEXT)
                .child(text),
        )
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
    use std::cell::{Cell, RefCell};
    use std::collections::HashMap;

    use gpui::{App, Entity, TestAppContext, VisualTestContext};
    use shell_data::EntryKind;

    use super::*;

    #[derive(Default)]
    struct Fake {
        listings: RefCell<HashMap<String, FolderListing>>,
        asked: RefCell<Vec<String>>,
        retried: RefCell<Vec<String>>,
        reachable: Cell<bool>,
    }

    impl FolderSource for Fake {
        fn host_name(&self) -> String {
            "Box".to_owned()
        }

        fn can_ask(&self) -> bool {
            self.reachable.get()
        }

        fn listing(
            &self,
            path: &str,
        ) -> Option<FolderListing> {
            self.listings.borrow().get(path).cloned()
        }

        fn request(
            &self,
            path: &str,
        ) {
            self.asked.borrow_mut().push(path.to_owned());
        }

        fn retry(
            &self,
            path: &str,
        ) {
            self.retried.borrow_mut().push(path.to_owned());
        }
    }

    type Events = Rc<RefCell<Vec<PickerEvent>>>;

    fn folder(path: &str) -> BrowseEntry {
        let name = path.rsplit('/').next().unwrap_or_default();
        BrowseEntry {
            name: name.to_owned(),
            path: path.to_owned(),
            kind: EntryKind::Folder,
        }
    }

    fn open<'a>(
        cx: &'a mut TestAppContext,
        fake: &Rc<Fake>,
        events: &Events,
    ) -> (Entity<RemoteFolderPicker>, &'a mut VisualTestContext) {
        let source = Rc::clone(fake);
        let (picker, cx) = cx.add_window_view(|_, _| RemoteFolderPicker::over(source));
        let sink = Rc::clone(events);
        cx.update(|_, cx| record(&picker, sink, cx));
        (picker, cx)
    }

    fn record(
        picker: &Entity<RemoteFolderPicker>,
        sink: Events,
        cx: &mut App,
    ) {
        cx.subscribe(picker, move |_, event: &PickerEvent, _| {
            sink.borrow_mut().push(event.clone());
        })
        .detach();
    }

    fn reachable_fake() -> Rc<Fake> {
        let fake = Rc::new(Fake::default());
        fake.reachable.set(true);
        let root = FolderListing::Loaded(vec![folder("/srv"), folder("/usr")]);
        fake.listings.borrow_mut().insert("/".to_owned(), root);
        fake
    }

    fn choosing_a_folder_opens_it_and_select_hands_over_its_path(cx: &mut TestAppContext) {
        let (fake, events) = (reachable_fake(), Events::default());
        let (picker, cx) = open(cx, &fake, &events);
        cx.run_until_parked();
        picker.update(cx, RemoteFolderPicker::confirm);
        assert_eq!(events.borrow().len(), 0, "nothing chosen yet");
        picker.update(cx, |this, cx| this.choose(&folder("/srv"), cx));
        cx.run_until_parked();
        assert!(
            fake.asked.borrow().contains(&"/srv".to_owned()),
            "the opened folder is listed"
        );
        picker.update(cx, RemoteFolderPicker::confirm);
        assert_eq!(*events.borrow(), [PickerEvent::Chosen("/srv".to_owned())]);
    }

    fn select_needs_a_host_that_answers_and_cancel_always_dismisses(cx: &mut TestAppContext) {
        let (fake, events) = (reachable_fake(), Events::default());
        let (picker, cx) = open(cx, &fake, &events);
        picker.update(cx, |this, cx| this.choose(&folder("/usr"), cx));
        fake.reachable.set(false);
        picker.update(cx, RemoteFolderPicker::confirm);
        assert_eq!(
            events.borrow().len(),
            0,
            "a host that does not answer is not chosen on"
        );
        picker.update(cx, |_, cx| RemoteFolderPicker::cancel(cx));
        assert_eq!(*events.borrow(), [PickerEvent::Cancelled]);
        picker.update(cx, |this, cx| this.toggle("/usr", cx));
        assert!(
            picker.read_with(cx, |this, _| this.expanded.is_empty()),
            "toggling closes it"
        );
    }

    fn the_root_failure_offers_a_retry_that_asks_again(cx: &mut TestAppContext) {
        let (fake, events) = (Rc::new(Fake::default()), Events::default());
        fake.reachable.set(true);
        fake.listings
            .borrow_mut()
            .insert("/".to_owned(), FolderListing::Failed("denied".into()));
        let (picker, cx) = open(cx, &fake, &events);
        cx.run_until_parked();
        picker.update(cx, RemoteFolderPicker::retry_root);
        assert_eq!(*fake.retried.borrow(), ["/".to_owned()]);
    }

    const CASES: [fn(&mut TestAppContext); 3] = [
        choosing_a_folder_opens_it_and_select_hands_over_its_path,
        select_needs_a_host_that_answers_and_cancel_always_dismisses,
        the_root_failure_offers_a_retry_that_asks_again,
    ];

    #[gpui::test]
    fn every_pick_ends_as_the_user_and_the_host_allow(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        for case in CASES {
            case(cx);
        }
    }
}
