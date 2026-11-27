use gpui::App;

pub(crate) fn quit_with_last_window(
    cx: &App,
    quit: impl Fn(&mut App) + 'static,
) {
    cx.on_window_closed(move |cx, _| {
        let last = cx.windows().is_empty();
        last.then(|| quit(cx));
    })
    .detach();
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;

    fn close(
        window: gpui::WindowHandle<gpui::Empty>,
        cx: &mut TestAppContext,
    ) {
        let removed = cx.update(|cx| window.update(cx, |_, window, _| window.remove_window()));
        removed.unwrap();
        cx.run_until_parked();
    }

    #[gpui::test]
    fn the_app_quits_once_its_last_window_closes(cx: &mut TestAppContext) {
        let quits = Rc::new(Cell::new(0));
        let heard = Rc::clone(&quits);
        cx.update(|cx| quit_with_last_window(cx, move |_| heard.set(heard.get() + 1)));
        let first = cx.add_window(|_, _| gpui::Empty);
        let second = cx.add_window(|_, _| gpui::Empty);
        close(first, cx);
        assert_eq!(quits.get(), 0, "a window is left");
        close(second, cx);
        assert_eq!(quits.get(), 1, "the last one closed");
    }
}
