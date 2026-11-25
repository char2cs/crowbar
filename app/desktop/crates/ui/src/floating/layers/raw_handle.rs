//! The platform handle of a window, which a float host places and shapes windows through.

use gpui::Window;
use raw_window_handle::{HasWindowHandle, RawWindowHandle};

pub(crate) fn raw_handle(window: &Window) -> Option<RawWindowHandle> {
    HasWindowHandle::window_handle(window)
        .ok()
        .map(|handle| handle.as_raw())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::raw_handle;

    #[gpui::test]
    fn a_test_window_has_no_handle_so_no_host_touches_it(cx: &mut TestAppContext) {
        let window = cx.add_empty_window();
        let raw = window.update(|window, _| raw_handle(window));
        assert_eq!(raw, None::<raw_window_handle::RawWindowHandle>);
    }
}
