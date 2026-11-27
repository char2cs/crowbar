use gpui::{
    App, DispatchPhase, EntityId, Hitbox, HitboxBehavior, IntoElement, ScrollDelta, ScrollEvent,
    ScrollHandle, Styled as _, Window, canvas, point, px,
};

const CLICK_LINES: f32 = 3.0;
const STEP_POWER: f32 = 2.0 / 3.0;

pub(super) fn wheel_step(handle: &ScrollHandle) -> impl IntoElement {
    let viewport = handle.clone();
    let scrolled = handle.clone();
    canvas(
        move |_, window, _| window.insert_hitbox(viewport.bounds(), HitboxBehavior::Normal),
        move |_, hitbox, window, _| listen(hitbox, scrolled, window),
    )
    .absolute()
}

fn listen(
    hitbox: Hitbox,
    handle: ScrollHandle,
    window: &mut Window,
) {
    let area = (hitbox, handle, window.current_view());
    window.on_mouse_event(move |event: &ScrollEvent, phase, window, cx| {
        on_wheel(&area, (event, phase), window, cx);
    });
}

fn on_wheel(
    area: &(Hitbox, ScrollHandle, EntityId),
    wheel: (&ScrollEvent, DispatchPhase),
    window: &Window,
    cx: &mut App,
) {
    let (hitbox, handle, view) = area;
    let (event, phase) = wheel;
    let ours = phase == DispatchPhase::Bubble && hitbox.should_handle_scroll(window);
    let Some(lines) = wheel_lines(event).filter(|_| ours) else {
        return;
    };
    scroll(handle, lines / CLICK_LINES * click_step(window));
    cx.notify(*view);
    cx.stop_propagation();
}

fn wheel_lines(event: &ScrollEvent) -> Option<f32> {
    match event.delta {
        ScrollDelta::Lines(lines) if lines.y != 0.0 => Some(lines.y),
        _ => None,
    }
}

fn click_step(window: &Window) -> f32 {
    f32::from(window.viewport_size().height)
        .powf(STEP_POWER)
        .floor()
}

fn scroll(
    handle: &ScrollHandle,
    by: f32,
) {
    let offset = handle.offset();
    let lowest = -f32::from(handle.max_offset().y);
    let y = (f32::from(offset.y) + by).clamp(lowest, 0.0);
    handle.set_offset(point(offset.x, px(y)));
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, div};

    use super::*;
    use crate::display::scroll_area::ScrollArea;

    struct Scroller(ScrollHandle);

    impl gpui::Render for Scroller {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut gpui::Context<'_, Self>,
        ) -> impl IntoElement {
            ScrollArea::new("sa", &self.0, div().h(px(10_000.0)))
        }
    }

    fn scrolled_by(
        delta: ScrollDelta,
        height: f32,
        cx: &mut TestAppContext,
    ) -> f32 {
        let handle = ScrollHandle::new();
        let shared = handle.clone();
        let (_, cx) = cx.add_window_view(|_, _| Scroller(shared));
        cx.simulate_resize(gpui::size(px(1280.0), px(height)));
        cx.run_until_parked();
        cx.simulate_event(ScrollEvent {
            position: point(px(10.0), px(10.0)),
            delta,
            ..ScrollEvent::default()
        });
        -f32::from(handle.offset().y)
    }

    #[gpui::test]
    fn a_wheel_click_scrolls_as_far_as_webkit_gtk_does(cx: &mut TestAppContext) {
        let click = ScrollDelta::Lines(point(0.0, -3.0));
        let scrolled = [800.0, 600.0].map(|height| scrolled_by(click, height, cx));
        assert_eq!(
            scrolled,
            [86.0, 71.0],
            "pow(h, 2/3) of the window per click"
        );
    }

    #[gpui::test]
    fn a_touchpad_scrolls_by_its_own_pixels(cx: &mut TestAppContext) {
        let swipe = ScrollDelta::Pixels(point(px(0.0), px(-40.0)));
        assert_eq!(scrolled_by(swipe, 800.0, cx), 40.0);
    }
}
