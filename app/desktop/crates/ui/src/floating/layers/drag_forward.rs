//! Hands a drag that moves off a float window's solid cards to the owner, where its drop
//! targets are.

use std::rc::Rc;

use gpui::{
    AnyWindowHandle, App, Bounds, DispatchPhase, MouseMoveEvent, MouseUpEvent, Pixels,
    PlatformInput, Point, Window,
};

use crate::floating::layers::float_region::FloatRegion;
use crate::floating::layers::part::Part;

pub(crate) struct DragForward {
    owner: AnyWindowHandle,
    cards: Vec<(Part, FloatRegion)>,
    scale: f32,
}

impl DragForward {
    pub(crate) fn listen(
        owner: AnyWindowHandle,
        cards: Vec<(Part, FloatRegion)>,
        window: &mut Window,
    ) {
        let scale = window.scale_factor();
        let moves = Rc::new(Self {
            owner,
            cards,
            scale,
        });
        let ups = Rc::clone(&moves);
        window.on_mouse_event(move |event: &MouseMoveEvent, phase, _, cx| {
            moves.pass(PlatformInput::MouseMove(event.clone()), phase, cx);
        });
        window.on_mouse_event(move |event: &MouseUpEvent, phase, _, cx| {
            ups.pass(PlatformInput::MouseUp(event.clone()), phase, cx);
        });
    }

    fn pass(
        &self,
        event: PlatformInput,
        phase: DispatchPhase,
        cx: &mut App,
    ) {
        let position = match &event {
            PlatformInput::MouseMove(event) => event.position,
            PlatformInput::MouseUp(event) => event.position,
            _ => return,
        };
        let dragging = phase == DispatchPhase::Capture && cx.has_active_drag();
        if !dragging || self.over_solid(position) {
            return;
        }
        cx.stop_propagation();
        let forwarded = self
            .owner
            .update(cx, |_, window, cx| window.dispatch_event(event, cx));
        if let Err(error) = forwarded {
            tracing::debug!(%error, "a drag left a float window whose owner closed");
        }
    }

    fn over_solid(
        &self,
        position: Point<Pixels>,
    ) -> bool {
        let at = Bounds::new(position, gpui::Size::default())
            .to_device_pixels(self.scale)
            .origin;
        self.cards
            .iter()
            .any(|(part, region)| *part == Part::Solid && region.bounds.contains(&at))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{
        AppContext as _, Context, InteractiveElement as _, IntoElement, ParentElement as _, Render,
        StatefulInteractiveElement as _, Styled as _, TestAppContext, Window, div, point, px,
    };

    use crate::Portal;
    use crate::test_support::probe::open_floating;
    use crate::testing::newest_window;

    #[derive(Clone)]
    struct Carried;

    struct Ghost;

    impl Render for Ghost {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div().size(px(4.0))
        }
    }

    fn grip() -> crate::Shaped {
        let handle = div()
            .id("grip")
            .size(px(10.0))
            .on_drag(Carried, |_, _, _, cx| cx.new(|_| Ghost));
        crate::Shaped::solid(handle, px(0.0))
    }

    #[gpui::test]
    fn a_drag_from_a_float_window_drops_on_its_owner(cx: &mut TestAppContext) {
        let dropped = Rc::new(Cell::new(0));
        let count = Rc::clone(&dropped);
        let mut owner = open_floating(cx, move |_, _| {
            let count = Rc::clone(&count);
            let grip = Portal::here("grip", |_, _| grip());
            div()
                .id("target")
                .size(px(300.0))
                .on_drop(move |_: &Carried, _, _| count.set(count.get() + 1))
                .child(div().size(px(10.0)).child(grip))
        });
        let mut top = newest_window(&mut owner);
        let (left, none) = (gpui::MouseButton::Left, gpui::Modifiers::none());
        top.simulate_mouse_down(point(px(5.0), px(5.0)), left, none);
        top.simulate_mouse_move(point(px(100.0), px(100.0)), left, none);
        top.simulate_mouse_move(point(px(120.0), px(100.0)), left, none);
        top.simulate_mouse_up(point(px(120.0), px(100.0)), left, none);
        top.run_until_parked();
        assert_eq!(dropped.get(), 1, "the owner holds the drop target");
        assert!(!cx.update(|cx| cx.has_active_drag()), "the drag ended");
    }
}
