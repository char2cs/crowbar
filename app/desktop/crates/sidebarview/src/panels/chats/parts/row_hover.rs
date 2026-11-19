//! Whether the pointer is over a tree row. Kept as state, not a group-hover style, because the
//! row's buttons take room from its label and gpui resolves group hover only after layout.

use gpui::{App, Div, ElementId, Entity, Stateful, StatefulInteractiveElement, Window};
use ui::Pose;

pub(crate) fn row_hover(
    id: &ElementId,
    window: &mut Window,
    cx: &mut App,
) -> Entity<Pose> {
    window.use_keyed_state((id.clone(), "hovered"), cx, |_, _| Pose::Off)
}

pub(crate) fn hovered_by(
    shell: Stateful<Div>,
    hover: &Entity<Pose>,
) -> Stateful<Div> {
    let hover = hover.clone();
    shell.on_hover(move |hovered, _, cx| set(&hover, Pose::from(*hovered), cx))
}

fn set(
    hover: &Entity<Pose>,
    pose: Pose,
    cx: &mut App,
) {
    hover.update(cx, |state, cx| {
        *state = pose;
        cx.notify();
    });
}

#[cfg(test)]
mod tests {
    use gpui::{
        InteractiveElement, Modifiers, ParentElement, Styled, TestAppContext, div, point, px,
    };

    use super::*;
    use crate::test_support::mount;

    #[gpui::test]
    fn the_row_remembers_the_pointer_coming_and_going(cx: &mut TestAppContext) {
        let seen = std::rc::Rc::new(std::cell::Cell::new(Pose::Off));
        let sink = std::rc::Rc::clone(&seen);
        let cx = mount(cx, move |window, cx| {
            let id = ElementId::from("row");
            let hover = row_hover(&id, window, cx);
            sink.set(*hover.read(cx));
            let shell = div().id(id).h(px(36.0)).child("row");
            hovered_by(shell, &hover)
        });
        cx.simulate_mouse_move(point(px(20.0), px(10.0)), None, Modifiers::none());
        cx.run_until_parked();
        let over = seen.get();
        cx.simulate_mouse_move(point(px(20.0), px(300.0)), None, Modifiers::none());
        cx.run_until_parked();
        assert_eq!((over, seen.get()), (Pose::On, Pose::Off));
    }
}
