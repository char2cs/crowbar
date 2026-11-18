//! The dialog: a search field over the remote branches of one repository, and the import button.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    AppContext as _, Context, Entity, EventEmitter, IntoElement, Render, Subscription, Window,
};
use shell_data::BranchImport;
use ui::{Dialog, DialogSize, Surface, TextInput, TextInputEvent, TextInputStyle, Theme};

use crate::branch_source::BranchSource;
use crate::import_event::ImportEvent;
use crate::row_look::{PLACEHOLDER, TITLE};
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

mod rows;

impl EventEmitter<ImportEvent> for BranchImportView {}

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
    use shell_data::{Availability, BranchListing, ImportBranch, ImportPlan};

    use super::*;

    pub(super) type Imported = Rc<RefCell<Vec<(Vec<String>, Vec<String>)>>>;

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

    pub(super) fn branch(
        name: &str,
        availability: Availability,
    ) -> ImportBranch {
        ImportBranch {
            name: name.to_owned(),
            availability,
        }
    }

    pub(super) fn open<'a>(
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
