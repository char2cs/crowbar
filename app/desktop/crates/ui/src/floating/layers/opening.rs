//! Opens an owner's float window of one kind as the stock window that kind is.

use gpui::{
    App, AppContext as _, Bounds, Entity, EntityId, Pixels, WindowBackgroundAppearance,
    WindowBounds, WindowDecorations, WindowKind, WindowOptions,
};

use crate::floating::layers::owner::Owner;
use crate::floating::layers::root::Root;
use crate::floating::portal::float::Float;
use crate::floating::portal::layer::Layer;

pub(crate) fn place(
    owner: &Entity<Owner>,
    (key, layer): ((EntityId, Float), Layer),
    cx: &mut App,
) {
    let opens = owner.update(cx, |owner, cx| owner.place(key, layer, cx));
    let held = owner.clone();
    if opens {
        cx.defer(move |cx| open(&held, key.1, cx));
    }
}

fn open(
    owner: &Entity<Owner>,
    float: Float,
    cx: &mut App,
) {
    let this = owner.read(cx);
    let opened = this.floats().iter().any(|(kind, _)| *kind == float);
    let unwanted = opened || this.shown(float).is_empty();
    let below = this.window();
    if unwanted {
        return;
    }
    let bounds = below
        .update(cx, |_, window, _| window.bounds())
        .unwrap_or_default();
    let held = owner.clone();
    let built = cx.open_window(options(float, bounds), move |window, cx| {
        cx.new(|cx| Root::new((held, float), window, cx))
    });
    match built {
        Ok(window) => owner.update(cx, |this, _| this.set_window(float, Some(window.into()))),
        Err(error) => tracing::warn!(%error, ?float, "a float window did not open"),
    }
}

fn options(
    float: Float,
    bounds: Bounds<Pixels>,
) -> WindowOptions {
    let (kind, focus) = match float {
        Float::Modal => (WindowKind::Dialog, true),
        Float::Popover => (WindowKind::Floating, true),
        Float::Passive | Float::Drag => (WindowKind::PopUp, false),
    };
    WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        titlebar: None,
        focus,
        show: true,
        kind,
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
    fn each_float_kind_opens_one_window_for_all_its_portals(cx: &mut TestAppContext) {
        let mut owner = open_floating(cx, |_, _| {
            div()
                .child(Portal::here("a", |_, _| floated()))
                .child(Portal::here("b", |_, _| floated()))
                .child(Portal::here("c", |_, _| floated()).float(Float::Popover))
        });
        assert_eq!(
            windows(&mut owner),
            3,
            "the owner, a passive and a popover window"
        );
    }
}
