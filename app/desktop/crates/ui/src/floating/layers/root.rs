//! The view of the pop-up window: its owner's passive layers, laid out as in the owner.

use gpui::{
    AnyElement, App, Context, Entity, IntoElement, ParentElement, Render, Styled, Subscription,
    WeakEntity, Window, canvas, div,
};
use raw_window_handle::RawWindowHandle;

use crate::WindowActivity;
use crate::floating::layers::float_host::float_region::FloatRegion;
use crate::floating::layers::float_host::part::Part;
use crate::floating::layers::float_host::raw_handle::raw_handle;
use crate::floating::layers::owner::Owner;
use crate::floating::layers::registry::Registry;
use crate::foundation::opaque_debug;

pub(crate) struct Root {
    owner: Entity<Owner>,
    raw: Option<RawWindowHandle>,
    _watching: Vec<Subscription>,
}

impl Root {
    pub(crate) fn new(
        owner: Entity<Owner>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let id = window.window_handle().window_id();
        Registry::start_frame(id, cx);
        cx.on_release(move |_, cx| Registry::forget_float(id, cx))
            .detach();
        let watching = vec![
            cx.observe(&owner, |root, _, cx| root.redraw(cx)),
            cx.observe_global::<WindowActivity>(Self::redraw),
        ];
        let root = Self {
            owner,
            raw: raw_handle(window),
            _watching: watching,
        };
        root.introduce(cx);
        root
    }

    fn introduce(
        &self,
        cx: &App,
    ) {
        let below = self.owner.read(cx).raw();
        let (Some(host), Some(raw), Some(below)) = (Registry::host(cx), self.raw, below) else {
            return;
        };
        host.cover(raw, below);
        host.shape(raw, &[], &[]);
    }

    fn redraw(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let (Some(host), Some(raw)) = (Registry::host(cx), self.raw) {
            host.wake(raw);
        }
        cx.notify();
    }

    fn settle(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        let (Some(host), Some(raw)) = (Registry::host(cx), self.raw) else {
            return;
        };
        let (seen, hit) = self.shape(window, cx);
        host.shape(raw, &seen, &hit);
    }

    fn shape(
        &self,
        window: &Window,
        cx: &mut App,
    ) -> (Vec<FloatRegion>, Vec<FloatRegion>) {
        if !self.pair_active(window, cx) {
            return (Vec::new(), Vec::new());
        }
        let cards = Registry::painted(window.window_handle().window_id(), cx);
        let seen = of(&cards, &[Part::Solid, Part::Seen]);
        let hit = of(&cards, &[Part::Solid, Part::Catch]);
        (seen, hit)
    }

    fn pair_active(
        &self,
        window: &Window,
        cx: &mut App,
    ) -> bool {
        let owner = self.owner.read(cx).window();
        let active = owner
            .update(cx, |_, window, cx| WindowActivity::of(window, cx))
            .unwrap_or_default();
        active || WindowActivity::of(window, cx)
    }
}

impl Render for Root {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        Registry::start_frame(window.window_handle().window_id(), cx);
        let layers: Vec<AnyElement> = self
            .owner
            .read(cx)
            .shown()
            .iter()
            .map(|layer| layer.paint(window, cx))
            .collect();
        let this = cx.weak_entity();
        let after = canvas(
            |_, _, _| {},
            move |_, (), window, cx| painted(&this, window, cx),
        );
        div()
            .size_full()
            .relative()
            .child(crate::hear())
            .children(layers)
            .child(after)
    }
}

opaque_debug!(Root);

fn painted(
    this: &WeakEntity<Root>,
    window: &mut Window,
    cx: &mut App,
) {
    let this = this.clone();
    window.defer(cx, move |window, cx| {
        this.update(cx, |root, cx| root.settle(window, cx))
            .unwrap_or_else(|error| tracing::debug!(%error, "the pop-up window closed first"));
    });
}

fn of(
    cards: &[(Part, FloatRegion)],
    parts: &[Part],
) -> Vec<FloatRegion> {
    cards
        .iter()
        .filter(|(part, _)| parts.contains(part))
        .map(|(_, region)| *region)
        .collect()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::floats::{floated, toggled, windows};
    use crate::test_support::probe::{open_floating, settle};
    use crate::testing::newest_window;
    use crate::{Float, Portal};

    #[gpui::test]
    fn a_pop_up_window_shows_its_cards_while_its_owner_s_keyboard_is_in_a_child_window(
        cx: &mut TestAppContext
    ) {
        let mut owner = open_floating(cx, |_, _| {
            Portal::here("p", |_, _| crate::Shaped::solid(floated(), gpui::px(4.0)))
        });
        let mut top = newest_window(&mut owner);
        owner.deactivate_window();
        top.deactivate_window();
        let id = owner.update(|window, _| window.window_handle().window_id());
        owner.update(|_, cx| crate::WindowActivity::gained(id, cx));
        settle(&mut owner);
        let seen = top.update(|window, cx| {
            let root = window.root::<super::Root>().flatten();
            root.map(|root| root.update(cx, |root, cx| root.shape(window, cx).0))
        });
        assert_eq!(seen.map(|seen| seen.len()), Some(1), "the toast stays seen");
    }

    #[gpui::test]
    fn a_pop_up_window_stays_open_once_its_last_portal_stops_rendering(cx: &mut TestAppContext) {
        let (shown, mut owner) = toggled(cx, Float::Passive);
        let mut top = newest_window(&mut owner);
        assert!(top.debug_bounds("floated").is_some());
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert_eq!(top.debug_bounds("floated"), None);
        assert_eq!(windows(&mut owner), 2, "kept for the next tooltip");
    }
}
