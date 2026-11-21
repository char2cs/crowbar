use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, AppContext as _, Div, Entity, InteractiveElement, IntoElement, MouseDownEvent, RenderOnce,
    SharedString, Stateful, StatefulInteractiveElement, Styled, Window,
};
use ui::DragGhost;

use crate::tabbar::strip_seat::StripSeat;
use crate::tabbar::strip_state::StripState;
use crate::tabbar::strip_tab::StripTab;
use crate::tabbar::tab_drag::TabDrag;
use crate::tabbar::tab_ghost::TabGhost;

const GHOST_WIDTH: f32 = 160.0;
const LIFTED_OPACITY: f32 = 0.4;

#[derive(IntoElement)]
pub(crate) struct CarriedTab {
    pub(crate) tab: StripTab,
    pub(crate) seat: StripSeat,
}

impl CarriedTab {
    pub(crate) fn seated(
        tabs: Vec<StripTab>,
        strip: &Entity<StripState>,
        cx: &App,
    ) -> Vec<Self> {
        let count = tabs.len();
        let order = strip
            .read(cx)
            .drag
            .as_ref()
            .map_or_else(|| (0..count).collect(), |drag| drag.order(count));
        let mut unseated: Vec<Option<StripTab>> = tabs.into_iter().map(Some).collect();
        order
            .into_iter()
            .filter_map(|index| Some((index, unseated.get_mut(index)?.take()?)))
            .map(|(index, tab)| Self::at(tab, (index, count), strip))
            .collect()
    }

    fn at(
        tab: StripTab,
        place: (usize, usize),
        strip: &Entity<StripState>,
    ) -> Self {
        let seat = StripSeat {
            pane: tab.pane.clone(),
            index: place.0,
            tabs: place.1,
            strip: strip.clone(),
        };
        Self { tab, seat }
    }
}

impl RenderOnce for CarriedTab {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let Self { tab, seat } = self;
        let lifted = seat
            .strip
            .read(cx)
            .drag
            .as_ref()
            .is_some_and(|drag| drag.lifts(seat.index));
        let label = tab.look.label.clone();
        tab.element(cx, move |frame| wire(frame, seat, label))
            .when(lifted, |tab| tab.opacity(LIFTED_OPACITY))
    }
}

fn wire(
    frame: Stateful<Div>,
    seat: StripSeat,
    label: SharedString,
) -> Stateful<Div> {
    let pressing = seat.strip.clone();
    frame
        .capture_any_mouse_down(move |event, _, cx| press(&pressing, event, cx))
        .on_drag(key_of(&seat), move |_, _, _, cx| {
            lift(&seat, label.clone(), cx)
        })
}

fn key_of(seat: &StripSeat) -> TabDrag {
    TabDrag {
        strip: seat.strip.entity_id(),
    }
}

fn press(
    strip: &Entity<StripState>,
    event: &MouseDownEvent,
    cx: &mut App,
) {
    strip.update(cx, |state, _| state.pressed = event.position);
}

fn lift(
    seat: &StripSeat,
    label: SharedString,
    cx: &mut App,
) -> Entity<TabGhost> {
    let tab = (seat.pane.clone(), seat.index);
    seat.strip
        .update(cx, |state, _| state.begin(tab, seat.tabs));
    let pill = cx.new(|_| DragGhost::new(label, GHOST_WIDTH));
    let strip = seat.strip.clone();
    cx.new(|_| TabGhost { pill, strip })
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;
    use protocol::PaneId;

    use super::*;
    use crate::tabbar::tab_actions::{TabAction, TabActions};
    use crate::tabbar::tab_end::TabEnd;
    use crate::tabbar::tab_look::TabLook;
    use crate::tabbar::tab_state::TabState;
    use crate::test_support::dragged;

    fn tab(name: &str) -> StripTab {
        let nothing: TabAction = Rc::new(|_, _| {});
        let actions = TabActions {
            select: Rc::clone(&nothing),
            close: Rc::clone(&nothing),
            remove: nothing,
            keep: None,
        };
        StripTab {
            pane: PaneId::new(name).expect("id"),
            look: TabLook::plain(name.to_owned()),
            state: TabState::Idle,
            end: TabEnd::Close,
            menu: Rc::from(Vec::new()),
            actions,
            ringed: false,
        }
    }

    fn seated_order(
        cx: &mut TestAppContext,
        drag: Option<crate::tabbar::strip_drag::StripDrag>,
    ) -> Vec<String> {
        let strip = cx.update(|cx| cx.new(StripState::new));
        strip.update(cx, |state, _| state.drag = drag);
        let tabs: Vec<StripTab> = ["a", "b", "c"].map(tab).into_iter().collect();
        let seated = cx.update(|cx| CarriedTab::seated(tabs, &strip, cx));
        seated
            .iter()
            .map(|carried| carried.seat.pane.as_str().to_owned())
            .collect()
    }

    #[gpui::test]
    fn the_bar_seats_its_tabs_in_the_order_a_lifted_tab_would_leave(cx: &mut TestAppContext) {
        let unmoved = ["a", "b", "c"];
        let cases = [
            ("no drag", None, unmoved),
            (
                "a press that has not lifted",
                Some(dragged(0, &[4.0])),
                unmoved,
            ),
            (
                "lifted past the last tab",
                Some(dragged(0, &[6.0, 260.0])),
                ["b", "c", "a"],
            ),
        ];
        for (name, drag, expected) in cases {
            assert_eq!(seated_order(cx, drag), expected, "{name}");
        }
    }
}
