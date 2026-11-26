//! Shows a passive layer, opening its owner's pop-up window, a stock window that never takes
//! the keyboard, on first use.

use gpui::{
    App, AppContext as _, Bounds, Entity, EntityId, Pixels, WindowBackgroundAppearance,
    WindowBounds, WindowDecorations, WindowKind, WindowOptions,
};

use crate::floating::layers::owner::Owner;
use crate::floating::layers::root::Root;
use crate::floating::portal::layer::Layer;

pub(crate) fn place(
    owner: &Entity<Owner>,
    (slot, layer): (EntityId, Layer),
    cx: &mut App,
) {
    let opens = owner.update(cx, |owner, cx| owner.place(slot, layer, cx));
    let held = owner.clone();
    if opens {
        cx.defer(move |cx| open(&held, cx));
    }
}

fn open(
    owner: &Entity<Owner>,
    cx: &mut App,
) {
    let this = owner.read(cx);
    if this.float().is_some() || this.shown().next().is_none() {
        return;
    }
    let bounds = this
        .window()
        .update(cx, |_, window, _| window.bounds())
        .unwrap_or_default();
    let held = owner.clone();
    let built = cx.open_window(options(bounds), move |window, cx| {
        cx.new(|cx| Root::new(held, window, cx))
    });
    match built {
        Ok(window) => owner.update(cx, |this, _| this.set_float(window.into())),
        Err(error) => tracing::warn!(%error, "the pop-up window did not open"),
    }
}

fn options(bounds: Bounds<Pixels>) -> WindowOptions {
    WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        titlebar: None,
        focus: false,
        show: true,
        kind: WindowKind::PopUp,
        is_movable: false,
        is_resizable: false,
        is_minimizable: false,
        window_background: WindowBackgroundAppearance::Transparent,
        window_decorations: Some(WindowDecorations::Client),
        ..WindowOptions::default()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement as _, TestAppContext, div};

    use crate::test_support::floats::{floated, windows};
    use crate::test_support::probe::open_floating;
    use crate::{Float, Portal};

    #[gpui::test]
    fn passive_portals_share_one_pop_up_window_and_a_popover_opens_none(cx: &mut TestAppContext) {
        let mut owner = open_floating(cx, |_, _| {
            div()
                .child(Portal::here("a", |_, _| floated()))
                .child(Portal::here("b", |_, _| floated()))
                .child(Portal::here("c", |_, _| floated()).float(Float::Popover))
        });
        assert_eq!(windows(&mut owner), 2, "the owner and its pop-up window");
    }
}
