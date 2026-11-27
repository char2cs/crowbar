use gpui::{
    App, DispatchPhase, Entity, IntoElement, MouseMoveEvent, MouseUpEvent, Pixels, Point, Styled,
    Window, canvas,
};

use crate::tabbar::strip_state::StripState;
use crate::tabbar::tab_actions::TabMove;

type Landing = (Entity<StripState>, TabMove);

pub(crate) fn probe(
    state: Entity<StripState>,
    reorder: TabMove,
) -> impl IntoElement {
    let landing: Landing = (state, reorder);
    canvas(
        |_, _, _| {},
        move |_, (), window, _| listen(landing, window),
    )
    .absolute()
    .size_0()
}

fn listen(
    landing: Landing,
    window: &mut Window,
) {
    let following = landing.0.clone();
    window.on_mouse_event(move |event: &MouseMoveEvent, phase, window, cx| {
        follow(&following, (event.position, phase), window, cx);
    });
    window.on_mouse_event(move |event: &MouseUpEvent, phase, window, cx| {
        land(&landing, (event.position, phase), window, cx);
    });
}

fn follow(
    state: &Entity<StripState>,
    moved: (Point<Pixels>, DispatchPhase),
    window: &mut Window,
    cx: &mut App,
) {
    let (pointer, phase) = moved;
    if phase != DispatchPhase::Capture {
        return;
    }
    let changed = state.update(cx, |state, _| state.dragged(pointer));
    if changed {
        state.update(cx, |_, cx| cx.notify());
    }
    if let Some(choose) = state.update(cx, |state, _| state.chosen_once_armed()) {
        choose(window, cx);
    }
}

fn land(
    landing: &Landing,
    release: (Point<Pixels>, DispatchPhase),
    window: &mut Window,
    cx: &mut App,
) {
    let (pointer, phase) = release;
    if phase != DispatchPhase::Capture {
        return;
    }
    let (state, reorder) = landing;
    if state.read(cx).drag.is_none() {
        return;
    }
    let (landed, chosen) = state.update(cx, |state, cx| {
        cx.notify();
        (state.land(pointer), state.chosen.take())
    });
    if let Some(choose) = chosen {
        choose(window, cx);
    }
    if let Some((pane, index)) = landed {
        reorder(&pane, index, window, cx);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{
        AppContext as _, Context, Modifiers, MouseButton, ParentElement, Render, TestAppContext,
        div, point, px,
    };

    use super::*;
    use crate::tabbar::drag::strip_drag::StripDrag;
    use crate::test_support::dragged;

    type Moves = Rc<RefCell<Vec<usize>>>;

    struct Bar {
        state: Entity<StripState>,
        moves: Moves,
    }

    impl Render for Bar {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let moves = Rc::clone(&self.moves);
            let reorder: TabMove = Rc::new(move |_, index, _, _| moves.borrow_mut().push(index));
            div().size_full().child(probe(self.state.clone(), reorder))
        }
    }

    fn released(
        cx: &mut TestAppContext,
        (drag, released_at): (StripDrag, Point<Pixels>),
    ) -> (Vec<usize>, bool) {
        let state = cx.update(|cx| cx.new(StripState::new));
        state.update(cx, |state, _| state.drag = Some(drag));
        let moves = Moves::default();
        let bar = (state.clone(), Rc::clone(&moves));
        let (_, cx) = cx.add_window_view(move |_, _| Bar {
            state: bar.0,
            moves: bar.1,
        });
        cx.run_until_parked();
        cx.simulate_mouse_up(released_at, MouseButton::Left, Modifiers::none());
        let ended = state.read_with(cx, |state, _| state.drag.is_none());
        (moves.take(), ended)
    }

    #[gpui::test]
    fn a_release_anywhere_lands_a_lifted_tab_and_ends_every_drag(cx: &mut TestAppContext) {
        let below_the_bar = |x: f32| point(px(x), px(300.0));
        let cases = [
            (
                "lifted past the last tab",
                dragged(0, &[6.0, 260.0]),
                310.0,
                vec![2],
            ),
            ("never lifted", dragged(0, &[4.0]), 54.0, vec![]),
            (
                "lifted only by the release itself",
                dragged(0, &[3.0]),
                310.0,
                vec![2],
            ),
        ];
        for (name, drag, x, expected) in cases {
            let ended = released(cx, (drag, below_the_bar(x)));
            assert_eq!(ended, (expected, true), "{name}");
        }
    }

    #[gpui::test]
    fn a_drag_lifted_since_the_last_paint_still_lands_on_release(cx: &mut TestAppContext) {
        let state = cx.update(|cx| cx.new(StripState::new));
        let moves = Moves::default();
        let bar = (state.clone(), Rc::clone(&moves));
        let (_, cx) = cx.add_window_view(move |_, _| Bar {
            state: bar.0,
            moves: bar.1,
        });
        cx.run_until_parked();
        state.update(cx, |state, _| state.drag = Some(dragged(0, &[6.0, 260.0])));
        cx.simulate_mouse_up(
            point(px(310.0), px(16.0)),
            MouseButton::Left,
            Modifiers::none(),
        );
        let ended = state.read_with(cx, |state, _| state.drag.is_none());
        assert_eq!((moves.take(), ended), (vec![2], true), "landed and ended");
    }
}
