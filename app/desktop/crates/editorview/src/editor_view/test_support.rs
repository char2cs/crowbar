//! Views opened for the unit tests, and reads of what they show.

use gpui::{App, Context, Entity, Focusable as _, TestAppContext, VisualTestContext, Window};

use super::EditorView;
use crate::find_widget::FindWidget;
use crate::{Access, EditorEvent, Opening};

pub(crate) fn opened<'cx>(
    (text, line): (&str, Option<u32>),
    access: Access,
    cx: &'cx mut TestAppContext,
) -> (Entity<EditorView>, &'cx mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).unwrap());
    cx.update(crate::init);
    let opening = Opening {
        text: text.to_owned(),
        path: "src/a.ts".to_owned(),
        line,
        access,
        saved: None,
    };
    cx.add_window_view(|window, cx| EditorView::new(opening, window, cx))
}

pub(crate) fn find_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> Entity<FindWidget> {
    window.read(|cx| view.read(cx).find.clone())
}

pub(crate) fn selected_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> std::ops::Range<usize> {
    window.read(|cx| view.read(cx).state.read(cx).selected_range())
}

pub(crate) fn rows_of(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> Option<std::ops::Range<usize>> {
    window.read(|cx| view.read(cx).state.read(cx).visible_row_range())
}

pub(crate) fn listened(
    view: &Entity<EditorView>,
    window: &mut VisualTestContext,
) -> std::rc::Rc<std::cell::RefCell<Vec<EditorEvent>>> {
    let heard = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
    let log = std::rc::Rc::clone(&heard);
    let record = move |_: Entity<EditorView>, event: &EditorEvent, _: &mut App| {
        log.borrow_mut().push(*event);
    };
    window.update(|window, cx| {
        window.focus(&view.focus_handle(cx), cx);
        cx.subscribe(view, record).detach();
    });
    heard
}

pub(crate) fn on_find<R>(
    find: &Entity<FindWidget>,
    window: &mut VisualTestContext,
    run: impl FnOnce(&mut FindWidget, &mut Window, &mut Context<'_, FindWidget>) -> R,
) -> R {
    window.update(|window, cx| find.update(cx, |find, cx| run(find, window, cx)))
}

pub(crate) fn open_find(
    find: &Entity<FindWidget>,
    reveal: crate::reveal::Reveal,
    window: &mut VisualTestContext,
) {
    on_find(find, window, |find, window, cx| {
        find.open(reveal, window, cx);
    });
}
