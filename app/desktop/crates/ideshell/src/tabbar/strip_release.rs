//! Ending a tab drag wherever the pointer is released, as develop's sortable bar lands a tab on
//! its closest slot even when the release is outside the bar.

use gpui::{App, DispatchPhase, Entity, IntoElement, MouseUpEvent, Styled, Window, canvas};

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
        move |_, (), window, cx| listen(landing, window, cx),
    )
    .absolute()
    .size_0()
}

fn listen(
    landing: Landing,
    window: &mut Window,
    cx: &App,
) {
    if landing.0.read(cx).drag.is_none() {
        return;
    }
    window.on_mouse_event(move |_: &MouseUpEvent, phase, window, cx| {
        land(&landing, phase, window, cx);
    });
}

fn land(
    landing: &Landing,
    phase: DispatchPhase,
    window: &mut Window,
    cx: &mut App,
) {
    if phase != DispatchPhase::Capture {
        return;
    }
    let (state, reorder) = landing;
    let landed = state.update(cx, |state, cx| {
        cx.notify();
        state.land()
    });
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
    use crate::tabbar::strip_drag::StripDrag;
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
        drag: StripDrag,
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
        let far = point(px(500.0), px(300.0));
        cx.simulate_mouse_up(far, MouseButton::Left, Modifiers::none());
        let ended = state.read_with(cx, |state, _| state.drag.is_none());
        (moves.take(), ended)
    }

    #[gpui::test]
    fn a_release_anywhere_lands_a_lifted_tab_and_ends_every_drag(cx: &mut TestAppContext) {
        let cases = [
            (
                "lifted past the last tab",
                dragged(0, &[6.0, 260.0]),
                vec![2],
            ),
            ("never lifted", dragged(0, &[4.0]), vec![]),
        ];
        for (name, drag, expected) in cases {
            assert_eq!(released(cx, drag), (expected, true), "{name}");
        }
    }
}
