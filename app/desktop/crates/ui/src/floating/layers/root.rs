//! The view of a float window: its owner's layers of one kind, laid out as in the owner.

use gpui::{
    AnyElement, App, Bounds, Context, DevicePixels, Entity, FocusHandle, InteractiveElement,
    IntoElement, ParentElement, Point, Render, Styled, Subscription, WeakEntity, Window, canvas,
    div,
};
use raw_window_handle::RawWindowHandle;

use crate::WindowActivity;
use crate::floating::layers::drag_forward::DragForward;
use crate::floating::layers::float_host::float_region::FloatRegion;
use crate::floating::layers::float_host::part::Part;
use crate::floating::layers::float_host::raw_handle::raw_handle;
use crate::floating::layers::owner::Owner;
use crate::floating::layers::registry::Registry;
use crate::floating::portal::float::Float;
use crate::foundation::opaque_debug;

pub(crate) struct Root {
    owner: Entity<Owner>,
    float: Float,
    raw: Option<RawWindowHandle>,
    focus: FocusHandle,
    _watching: Vec<Subscription>,
}

impl Root {
    pub(crate) fn new(
        (owner, float): (Entity<Owner>, Float),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let id = window.window_handle().window_id();
        Registry::start_frame(id, cx);
        cx.on_release(move |_, cx| Registry::forget_float(id, cx))
            .detach();
        let watching = vec![
            cx.observe_in(&owner, window, |this, _, window, cx| {
                this.follow_owner(window, cx);
            }),
            cx.observe_window_activation(window, |this, _, cx| {
                this.owner.update(cx, |_, cx| cx.notify());
            }),
        ];
        let root = Self {
            owner,
            float,
            raw: raw_handle(window),
            focus: cx.focus_handle(),
            _watching: watching,
        };
        root.introduce(window, cx);
        root
    }

    fn introduce(
        &self,
        window: &mut Window,
        cx: &App,
    ) {
        if self.float.takes_keys() {
            window.activate_window();
        }
        let below = self.owner.read(cx).raw();
        let (Some(host), Some(raw), Some(below)) = (Registry::host(cx), self.raw, below) else {
            return;
        };
        host.cover(raw, below, self.float);
        host.shape(raw, &[], &[]);
        if self.float == Float::Popover {
            window.on_next_frame(move |_, _| host.focus(raw));
        }
    }

    fn follow_owner(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let float = self.float;
        let emptied = float != Float::Passive && self.owner.read(cx).shown(float).is_empty();
        if !emptied {
            return cx.notify();
        }
        self.hand_back_focus(window, cx);
        self.owner
            .update(cx, |owner, _| owner.set_window(float, None));
        window.remove_window();
        let took_focus = float.takes_keys();
        let owner = self.owner.read(cx).window();
        if took_focus {
            owner
                .update(cx, |_, window, _| window.activate_window())
                .unwrap_or_else(|error| tracing::debug!(%error, "the owner closed first"));
        }
    }

    fn hand_back_focus(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        let Some(here) = window.focused(cx) else {
            return;
        };
        if self.focus.contains(&here, window) {
            return;
        }
        window.blur(cx);
        let owner = self.owner.read(cx).window();
        if let Err(error) = owner.update(cx, |_, window, cx| window.focus(&here, cx)) {
            tracing::debug!(%error, "the owner of a float window closed first");
        }
    }

    fn take_owner_focus(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        let owner = self.owner.read(cx).window();
        let focused = owner
            .update(cx, |_, window, cx| window.focused(cx))
            .ok()
            .flatten();
        let Some(focus) = focused.filter(|focus| self.focus.contains(focus, window)) else {
            return;
        };
        window.focus(&focus, cx);
        if let Err(error) = owner.update(cx, |_, window, cx| window.blur(cx)) {
            tracing::debug!(%error, "the owner of a float window closed first");
        }
    }

    fn settle(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        if self.float.takes_keys() {
            self.hand_back_focus(window, cx);
            self.take_owner_focus(window, cx);
        }
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
        if self.float == Float::Passive && !self.pair_active(window, cx) {
            return (Vec::new(), Vec::new());
        }
        let cards = Registry::painted(window.window_handle().window_id(), cx);
        let seen = of(&cards, &[Part::Solid, Part::Seen]);
        let hit = match self.float {
            Float::Modal => vec![whole(window)],
            Float::Drag => Vec::new(),
            Float::Popover | Float::Passive => of(&cards, &[Part::Solid, Part::Catch]),
        };
        (seen, hit)
    }

    fn pair_active(
        &self,
        window: &Window,
        cx: &mut App,
    ) -> bool {
        let owner = self.owner.read(cx);
        let mut windows = vec![owner.window()];
        windows.extend(owner.floats().into_iter().map(|(_, float)| float));
        let active = |other: gpui::AnyWindowHandle, cx: &mut App| {
            other
                .update(cx, |_, window, cx| WindowActivity::of(window, cx))
                .unwrap_or_default()
        };
        WindowActivity::of(window, cx) || windows.into_iter().any(|other| active(other, cx))
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
            .shown(self.float)
            .iter()
            .map(|layer| layer.paint(window, cx))
            .collect();
        let (this, owner) = (cx.weak_entity(), self.owner.read(cx).window());
        let after = canvas(
            |_, _, _| {},
            move |_, (), window, cx| painted((&this, owner), window, cx),
        );
        div()
            .size_full()
            .relative()
            .track_focus(&self.focus)
            .child(crate::hear())
            .children(layers)
            .child(after)
    }
}

opaque_debug!(Root);

fn painted(
    (this, owner): (&WeakEntity<Root>, gpui::AnyWindowHandle),
    window: &mut Window,
    cx: &mut App,
) {
    let cards = Registry::painted(window.window_handle().window_id(), cx);
    DragForward::listen(owner, cards, window);
    let this = this.clone();
    window.defer(cx, move |window, cx| {
        this.update(cx, |root, cx| root.settle(window, cx))
            .unwrap_or_else(|error| tracing::debug!(%error, "a float window closed first"));
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

fn whole(window: &Window) -> FloatRegion {
    let size = window
        .viewport_size()
        .to_device_pixels(window.scale_factor());
    FloatRegion {
        bounds: Bounds::new(Point::default(), size),
        radius: DevicePixels(0),
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{ParentElement as _, TestAppContext, div, prelude::FluentBuilder as _};

    use crate::test_support::floats::{floated, toggled, windows};
    use crate::test_support::probe::{open_floating, settle};
    use crate::testing::newest_window;
    use crate::{Float, Portal};

    #[gpui::test]
    fn a_passive_window_shows_its_cards_while_its_owner_s_keyboard_is_in_a_child_window(
        cx: &mut TestAppContext
    ) {
        let mut owner = open_floating(cx, |_, _| {
            Portal::here("p", |_, _| crate::Shaped::solid(floated(), gpui::px(4.0)))
                .float(Float::Passive)
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
    fn a_passive_window_stays_open_once_its_last_portal_stops_rendering(cx: &mut TestAppContext) {
        let (shown, mut owner) = toggled(cx, Float::Passive);
        let mut top = newest_window(&mut owner);
        assert!(top.debug_bounds("floated").is_some());
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert_eq!(top.debug_bounds("floated"), None);
        assert_eq!(windows(&mut owner), 2, "kept for the next tooltip");
    }

    fn menu() -> Portal {
        Portal::here("p", |_, _| floated()).float(Float::Popover)
    }

    #[gpui::test]
    fn a_popover_window_holds_the_activation_until_it_closes(cx: &mut TestAppContext) {
        let shown = Rc::new(Cell::new(false));
        let read = Rc::clone(&shown);
        let mut owner = open_floating(cx, move |_, _| {
            div().when(read.get(), |this| this.child(menu()))
        });
        let active = |owner: &mut gpui::VisualTestContext| {
            owner.update(|window, _| window.is_window_active())
        };
        owner.update(|window, _| window.activate_window());
        settle(&mut owner);
        shown.set(true);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert!(!active(&mut owner), "the popover window took it");
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert!(active(&mut owner));
    }
}
