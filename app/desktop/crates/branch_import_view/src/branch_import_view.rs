//! The dialog: a search field over the remote branches of one repository, and the import button.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    AppContext as _, Context, Div, Entity, EventEmitter, IntoElement, ParentElement, Render,
    SharedString, Styled, Subscription, Window, div, uniform_list,
};
use shell_data::{BranchImport, BranchListing, ImportBranch, ImportPlan};
use ui::{
    Button, ButtonSize, Dialog, DialogSize, Palette, Pose, Surface, TextInput, TextInputEvent,
    TextInputStyle, Theme,
};

use crate::branch_row::branch_row;
use crate::branch_source::BranchSource;
use crate::import_event::ImportEvent;
use crate::import_hint;
use crate::row_choice::{RowChoice, Toggle};
use crate::row_look::{
    FAILED, GAP, LEAD, LIST_HEIGHT, LOADING, NONE_MATCH, NONE_ON_REMOTE, PLACEHOLDER, TEXT, TITLE,
};
use crate::selection::Selection;

/// The branch import dialog over a source of branches; it emits [`ImportEvent::Close`] when done.
pub struct BranchImportView {
    source: Rc<dyn BranchSource>,
    input: Entity<TextInput>,
    filter: String,
    selection: Selection,
    _input_events: Subscription,
}

impl std::fmt::Debug for BranchImportView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("BranchImportView")
            .finish_non_exhaustive()
    }
}

impl EventEmitter<ImportEvent> for BranchImportView {}

impl BranchSource for Arc<BranchImport> {
    fn listing(&self) -> BranchListing {
        BranchImport::listing(self)
    }

    fn plan(
        &self,
        selected: &[String],
    ) -> ImportPlan {
        BranchImport::plan(self, selected)
    }

    fn import(
        &self,
        branches: &[String],
        locked: &[String],
    ) {
        BranchImport::import(self, branches, locked);
    }
}

fn matching(
    listing: &BranchListing,
    filter: &str,
) -> Vec<ImportBranch> {
    let BranchListing::Ready(rows) = listing else {
        return Vec::new();
    };
    let query = filter.to_lowercase();
    rows.iter()
        .filter(|row| row.name.to_lowercase().contains(&query))
        .cloned()
        .collect()
}

fn empty_message(
    listing: &BranchListing,
    shown: usize,
) -> Option<&'static str> {
    match listing {
        BranchListing::Loading => Some(LOADING),
        BranchListing::Failed => Some(FAILED),
        BranchListing::Ready(rows) if rows.is_empty() => Some(NONE_ON_REMOTE),
        BranchListing::Ready(_) if shown == 0 => Some(NONE_MATCH),
        BranchListing::Ready(_) => None,
    }
}

fn import_button(
    picked: usize,
    cx: &mut Context<'_, BranchImportView>,
) -> Button {
    let button = Button::new("import-start", "Import")
        .size(ButtonSize::Sm)
        .on_click(cx.listener(|this, _, _, cx| this.start(cx)));
    if picked == 0 {
        return button.disabled();
    }
    button
}

fn pick_handler(
    view: &Entity<BranchImportView>,
    name: &str,
) -> Toggle {
    let (view, name) = (view.clone(), name.to_owned());
    Box::new(move |_, cx| view.update(cx, |this, cx| this.pick(&name, cx)))
}

fn lock_handler(
    view: &Entity<BranchImportView>,
    name: &str,
) -> Toggle {
    let (view, name) = (view.clone(), name.to_owned());
    Box::new(move |_, cx| view.update(cx, |this, cx| this.lock(&name, cx)))
}

fn muted(palette: &Palette) -> Div {
    div().text_size(TEXT).text_color(palette.muted_foreground())
}

impl BranchImportView {
    /// A dialog over `import` with the search field focused.
    pub fn new(
        import: Arc<BranchImport>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self::over(Rc::new(import), window, cx)
    }

    fn over(
        source: Rc<dyn BranchSource>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let input = cx.new(|cx| {
            TextInput::new(PLACEHOLDER, cx)
                .on(Surface::Window)
                .style(TextInputStyle::Compact)
        });
        input.read(cx).focus(window);
        let subscription = cx.subscribe(&input, Self::on_input);
        Self {
            source,
            input,
            filter: String::new(),
            selection: Selection::default(),
            _input_events: subscription,
        }
    }

    fn on_input(
        &mut self,
        _: Entity<TextInput>,
        event: &TextInputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(query) => {
                self.filter.clone_from(query);
                cx.notify();
            },
            TextInputEvent::Cancelled => cx.emit(ImportEvent::Close),
            _ => {},
        }
    }

    fn pick(
        &mut self,
        name: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.selection.toggle(name);
        cx.notify();
    }

    fn lock(
        &mut self,
        name: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.selection.toggle_lock(name);
        cx.notify();
    }

    fn start(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let picked = self.selection.picked();
        if picked.is_empty() {
            return;
        }
        self.source.import(&picked, &self.selection.locked());
        cx.emit(ImportEvent::Close);
    }

    fn row_at(
        &self,
        branch: &ImportBranch,
        (index, palette): (usize, &Palette),
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let view = cx.entity();
        let choice = RowChoice {
            picked: Pose::from(self.selection.is_picked(&branch.name)),
            locked: Pose::from(self.selection.is_locked(&branch.name)),
            toggle: pick_handler(&view, &branch.name),
            toggle_lock: lock_handler(&view, &branch.name),
        };
        branch_row(index, branch, (choice, palette)).font_family(Theme::of(cx).mono_family())
    }

    fn row_if(
        &self,
        rows: &[ImportBranch],
        (index, palette): (usize, &Palette),
        cx: &mut Context<'_, Self>,
    ) -> Option<Div> {
        let row = rows.get(index)?;
        Some(self.row_at(row, (index, palette), cx))
    }

    fn list(
        rows: Vec<ImportBranch>,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let count = rows.len();
        let palette = *palette;
        uniform_list(
            "import-branches",
            count,
            cx.processor(move |this, range: std::ops::Range<usize>, _, cx| {
                range
                    .filter_map(|index| this.row_if(&rows, (index, &palette), cx))
                    .collect::<Vec<_>>()
            }),
        )
        .h(LIST_HEIGHT)
    }

    fn body(
        &self,
        palette: &Palette,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let listing = self.source.listing();
        let rows = matching(&listing, &self.filter);
        let message = empty_message(&listing, rows.len());
        let plan = self.source.plan(&self.selection.picked());
        let shown = match message {
            Some(text) => muted(palette).h(LIST_HEIGHT).child(text).into_any_element(),
            None => Self::list(rows, palette, cx).into_any_element(),
        };
        div()
            .flex()
            .flex_col()
            .gap(GAP)
            .child(muted(palette).child(LEAD))
            .child(self.input.clone())
            .child(shown)
            .child(muted(palette).child(SharedString::from(import_hint::of(plan))))
            .child(import_button(self.selection.picked().len(), cx))
    }
}

impl Render for BranchImportView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let me = cx.entity();
        Dialog::new("branch-import")
            .title(TITLE)
            .size(DialogSize::Md)
            .body(self.body(&palette, cx))
            .on_close(move |_, cx| me.update(cx, |_, cx| cx.emit(ImportEvent::Close)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{TestAppContext, VisualTestContext};
    use shell_data::Availability;

    use super::*;

    type Imported = Rc<RefCell<Vec<(Vec<String>, Vec<String>)>>>;

    struct Fake {
        listing: BranchListing,
        imported: Imported,
    }

    impl BranchSource for Fake {
        fn listing(&self) -> BranchListing {
            self.listing.clone()
        }

        fn plan(
            &self,
            selected: &[String],
        ) -> ImportPlan {
            ImportPlan {
                import_count: selected.len(),
                parent_count: 0,
            }
        }

        fn import(
            &self,
            branches: &[String],
            locked: &[String],
        ) {
            self.imported
                .borrow_mut()
                .push((branches.to_vec(), locked.to_vec()));
        }
    }

    fn branch(
        name: &str,
        availability: Availability,
    ) -> ImportBranch {
        ImportBranch {
            name: name.to_owned(),
            availability,
        }
    }

    fn open<'a>(
        cx: &'a mut TestAppContext,
        listing: BranchListing,
        imported: &Imported,
    ) -> (Entity<BranchImportView>, &'a mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let source = Rc::new(Fake {
            listing,
            imported: Rc::clone(imported),
        });
        cx.add_window_view(|window, cx| BranchImportView::over(source, window, cx))
    }

    type Message = (&'static str, usize, Option<&'static str>);

    const MESSAGES: [Message; 4] = [
        ("loading", 0, Some(LOADING)),
        ("failed", 0, Some(FAILED)),
        ("no filter match", 0, Some(NONE_MATCH)),
        ("rows shown", 2, None),
    ];

    #[test]
    fn the_list_says_why_it_is_empty() {
        let ready = BranchListing::Ready(vec![
            branch("a", Availability::Importable),
            branch("b", Availability::Importable),
        ]);
        let listings = [
            BranchListing::Loading,
            BranchListing::Failed,
            ready.clone(),
            ready,
        ];
        for ((name, shown, expected), listing) in MESSAGES.into_iter().zip(&listings) {
            assert_eq!(empty_message(listing, shown), expected, "{name}");
        }
        assert_eq!(
            empty_message(&BranchListing::Ready(Vec::new()), 0),
            Some(NONE_ON_REMOTE)
        );
    }

    #[test]
    fn the_filter_keeps_the_branches_whose_name_contains_it_in_any_case() {
        let listing = BranchListing::Ready(vec![
            branch("Feature-A", Availability::Importable),
            branch("main", Availability::Protected),
        ]);
        let names: Vec<_> = matching(&listing, "feat")
            .into_iter()
            .map(|row| row.name)
            .collect();
        assert_eq!(names, ["Feature-A"]);
        assert_eq!(matching(&BranchListing::Loading, "").len(), 0);
    }

    #[gpui::test]
    fn importing_hands_over_the_picked_and_locked_branches_and_closes(cx: &mut TestAppContext) {
        let imported = Imported::default();
        let listing = BranchListing::Ready(vec![branch("a", Availability::Importable)]);
        let (view, cx) = open(cx, listing, &imported);
        let closed = Rc::new(RefCell::new(0_usize));
        let count = Rc::clone(&closed);
        cx.update(|_, cx| {
            cx.subscribe(&view, move |_, _: &ImportEvent, _| *count.borrow_mut() += 1)
                .detach();
        });
        view.update(cx, BranchImportView::start);
        assert_eq!(imported.borrow().len(), 0, "nothing picked, nothing sent");
        view.update(cx, |view, cx| {
            view.pick("a", cx);
            view.lock("a", cx);
            view.start(cx);
        });
        assert_eq!(
            *imported.borrow(),
            [(vec!["a".to_owned()], vec!["a".to_owned()])]
        );
        assert_eq!(*closed.borrow(), 1);
    }
}
