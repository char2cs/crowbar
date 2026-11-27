use gpui::{App, Context, DispatchPhase, Entity, MouseMoveEvent, Pixels, Point, Window};

pub(crate) trait SelectionDrag: Sized + 'static {
    fn selecting(&self) -> bool;
    fn drag_selection(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    );
}

pub(crate) fn capture_pointer<S: SelectionDrag>(
    field: &Entity<S>,
    window: &mut Window,
    cx: &App,
) {
    if !field.read(cx).selecting() {
        return;
    }
    let field = field.clone();
    window.on_mouse_event(move |event: &MouseMoveEvent, phase, _, cx| {
        follow(&field, (event.position, phase), cx);
    });
}

fn follow<S: SelectionDrag>(
    field: &Entity<S>,
    (position, phase): (Point<Pixels>, DispatchPhase),
    cx: &mut App,
) {
    if phase.bubble() {
        field.update(cx, |field, cx| field.drag_selection(position, cx));
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, point, px};

    use crate::control::editor::test_support::field::{field, text_of};

    #[gpui::test]
    fn a_drag_that_leaves_the_field_still_selects_to_the_edge_it_left_by(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc def");
        cx.simulate_mouse_down(
            point(px(11.0), px(16.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_move(
            point(px(600.0), px(300.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_mouse_up(
            point(px(600.0), px(300.0)),
            gpui::MouseButton::Left,
            Modifiers::none(),
        );
        cx.simulate_input("Z");
        assert_eq!(text_of(&input, cx), "Z");
    }
}
