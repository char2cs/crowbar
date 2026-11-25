//! Float windows in tests of the crates that build on this one.
#![cfg(any(test, feature = "test-support"))]

use gpui::{App, VisualTestContext};
use raw_window_handle::RawWindowHandle;

use crate::floating::Owners;
use crate::{Float, FloatHost, FloatRegion};

impl FloatHost for () {
    fn cover(
        &self,
        _: RawWindowHandle,
        _: RawWindowHandle,
        _: Float,
    ) {
    }

    fn shape(
        &self,
        _: RawWindowHandle,
        _: &[FloatRegion],
        _: &[FloatRegion],
    ) {
    }

    fn focus(
        &self,
        _: RawWindowHandle,
    ) {
    }
}

/// Lets floating elements open their float windows as plain test windows, as the shell's
/// placer does on the desktop.
pub fn open_float_windows(cx: &mut App) {
    crate::install_float_host((), cx);
}

/// The float window `owner` opened last; `owner` itself while it has none.
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

/// The float window of kind `float` that `owner` has open; `owner` itself while it has none.
pub fn float_window(
    owner: &mut VisualTestContext,
    float: Float,
) -> VisualTestContext {
    owner.run_until_parked();
    let own = owner.update(|window, _| window.window_handle());
    let open = owner.update(|window, cx| {
        let floats = Owners::of(window, cx).map(|of| of.read(cx).floats());
        floats
            .unwrap_or_default()
            .into_iter()
            .find(|(kind, _)| *kind == float)
    });
    let window = open.map_or(own, |(_, window)| window);
    VisualTestContext::from_window(window, owner)
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
