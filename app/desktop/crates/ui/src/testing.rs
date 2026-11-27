#![cfg(any(test, feature = "test-support"))]

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, Bounds, Pixels, Task, TestAppContext, VisualTestContext};
use raw_window_handle::RawWindowHandle;

use crate::{FloatHost, FloatRegion, MenuPath, NativeEntry, NativeMenu, NativeRequest, PopupMenu};

impl FloatHost for () {
    fn cover(
        &self,
        _: RawWindowHandle,
        _: RawWindowHandle,
    ) {
    }

    fn wake(
        &self,
        _: RawWindowHandle,
    ) {
    }

    fn shape(
        &self,
        _: RawWindowHandle,
        _: &[FloatRegion],
        _: &[FloatRegion],
    ) {
    }
}

pub fn open_float_windows(cx: &mut App) {
    crate::install_float_host((), cx);
}

pub type Popped = Rc<RefCell<Vec<NativeRequest>>>;

pub fn answer_menus(
    cx: &mut TestAppContext,
    pick: impl Fn(&[NativeEntry]) -> Option<MenuPath> + 'static,
) -> Popped {
    let popped = Popped::default();
    let heard = Rc::clone(&popped);
    let popup: PopupMenu = Rc::new(move |request, _, _| {
        let path = pick(&request.entries);
        heard.borrow_mut().push(request);
        Task::ready(path)
    });
    cx.update(|cx| cx.set_global(NativeMenu::new(popup)));
    popped
}

#[must_use]
pub fn entry_labelled(
    entries: &[NativeEntry],
    label: &str,
) -> Option<MenuPath> {
    let is_it = |entry: &NativeEntry| matches!(entry, NativeEntry::Item { label: shown, .. } if shown == label);
    entries.iter().position(is_it).map(|at| vec![at])
}

pub fn newest_window(owner: &mut VisualTestContext) -> VisualTestContext {
    owner.run_until_parked();
    let own = owner.update(|window, _| window.window_handle());
    let newest = owner
        .update(|_, cx| cx.windows())
        .into_iter()
        .max_by_key(|window| window.window_id().as_u64())
        .unwrap_or(own);
    VisualTestContext::from_window(newest, owner)
}

pub fn covered(window: &mut VisualTestContext) -> (bool, Vec<Bounds<Pixels>>) {
    window.run_until_parked();
    window.update(|window, cx| {
        let cover = crate::Cover::of(window, cx);
        let cover = cover.read(cx);
        (cover.whole(), cover.cards())
    })
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement as _, TestAppContext, div};

    use super::*;
    use crate::Portal;
    use crate::test_support::floats::floated;
    use crate::test_support::probe::open_floating;

    #[gpui::test]
    fn the_newest_window_is_the_owner_until_a_portal_opens_one(cx: &mut TestAppContext) {
        let mut owner = open_floating(cx, |_, _| div());
        let own = owner.update(|window, _| window.window_handle().window_id());
        let alone =
            newest_window(&mut owner).update(|window, _| window.window_handle().window_id());
        assert_eq!(alone, own);
        let mut floating =
            open_floating(cx, |_, _| div().child(Portal::here("p", |_, _| floated())));
        let mut top = newest_window(&mut floating);
        assert!(top.debug_bounds("floated").is_some());
    }
}
