//! The cover of each open window, dropped with the window.

use std::collections::HashMap;

use gpui::{App, AppContext as _, Entity, Global, Subscription, Window, WindowId};

use crate::floating::portal::cover::Cover;

#[derive(Default)]
struct Covers {
    windows: HashMap<WindowId, (Entity<Cover>, Subscription)>,
}

impl Global for Covers {}

pub(super) fn of(
    window: &Window,
    cx: &mut App,
) -> Entity<Cover> {
    let id = window.window_handle().window_id();
    let known = cx
        .try_global::<Covers>()
        .and_then(|covers| covers.windows.get(&id));
    if let Some((cover, _)) = known {
        return cover.clone();
    }
    let cover = cx.new(|_| Cover::default());
    let closed = cx.on_window_closed(move |cx, closed| {
        cx.default_global::<Covers>().windows.remove(&closed);
    });
    let entry = (cover.clone(), closed);
    cx.default_global::<Covers>().windows.insert(id, entry);
    cover
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, div};

    use crate::test_support::probe::open;

    use super::*;

    #[gpui::test]
    fn a_window_keeps_one_cover(cx: &mut TestAppContext) {
        let window = open(cx, |_, _| div());
        let [first, second] = [(), ()].map(|()| window.update(|window, cx| of(window, cx)));
        assert_eq!(first, second, "the same cover each time");
    }
}
