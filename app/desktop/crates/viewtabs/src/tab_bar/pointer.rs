use gpui::{
    App, Context, Entity, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent, Pixels, Point,
    Window,
};
use layout::Command;
use ui::Motion;

use crate::event::Event;
use crate::gesture::DragSession;
use crate::gesture::Press;
use crate::gesture::Settle;
use crate::gesture::StripDrag;
use crate::strip_metrics;
use crate::tab_bar::TabBar;

pub(crate) fn follow_pointer(
    bar: &Entity<TabBar>,
    window: &mut Window,
    cx: &App,
) {
    let state = bar.read(cx);
    if state.press.is_none() && state.drag.is_none() {
        return;
    }
    let bar = bar.clone();
    window.on_mouse_event(move |event: &MouseMoveEvent, phase, window, cx| {
        phase
            .bubble()
            .then(|| bar.update(cx, |bar, cx| bar.moved(event, window, cx)));
    });
}

impl TabBar {
    pub(crate) fn pressed(
        &mut self,
        index: usize,
        event: &MouseDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if event.button != MouseButton::Left {
            return;
        }
        self.press = Some(Press {
            index,
            origin: event.position,
        });
        cx.notify();
    }

    pub(crate) fn moved(
        &mut self,
        event: &MouseMoveEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if event.pressed_button != Some(MouseButton::Left) {
            self.cancel_press(cx);
            return;
        }
        if self.drag.is_some() {
            self.drag_to(event.position, cx);
            return;
        }
        let Some(press) = self
            .press
            .filter(|press| press.has_moved_past_threshold(event.position))
        else {
            return;
        };
        self.begin_drag(press, event.position, cx);
    }

    pub(crate) fn released(
        &mut self,
        event: &MouseUpEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let press = self.press.take();
        self.incoming = None;
        let Some(drag) = self.drag.take() else {
            self.activate(press, cx);
            return;
        };
        self.settle_into_slot(&drag, cx);
        cx.notify();
        if let Some(dropped) = dropped_out(&drag, event.position) {
            cx.emit(dropped);
            return;
        }
        if let Some(command) = drag.engine.drop() {
            cx.emit(Event::Apply(vec![command]));
        }
    }

    fn drag_to(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        let x = self.content_x(position);
        let crossing = self.crossing(position);
        if let Some(drag) = &mut self.drag {
            drag.aim(x);
            drag.follow(position, now, motion);
        }
        cx.notify();
        if let Some(crossing) = crossing {
            cx.emit(crossing);
        }
    }

    fn crossing(
        &mut self,
        position: Point<Pixels>,
    ) -> Option<Event> {
        let outside = !self.row.contains(&position);
        let drag = self.drag.as_mut()?;
        let tab = drag.engine.tab()?;
        if outside {
            drag.out = true;
            return Some(Event::DragOut { tab, position });
        }
        std::mem::take(&mut drag.out).then_some(Event::DragCancel)
    }

    fn activate(
        &mut self,
        press: Option<Press>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = press.and_then(|press| self.tabs.get(press.index)) else {
            return;
        };
        let command = Command::ActivateTab { tab: tab.id };
        cx.emit(Event::Apply(vec![command]));
    }

    pub(crate) fn content_x(
        &self,
        position: Point<Pixels>,
    ) -> f32 {
        let scrolled = self.scroll.offset().x;
        f32::from(position.x - self.strip_left() - scrolled)
    }

    fn cancel_press(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let drag = self.drag.take();
        if self.press.take().is_some() || drag.is_some() {
            cx.notify();
        }
        if drag.is_some_and(|drag| drag.out) {
            cx.emit(Event::DragCancel);
        }
    }

    fn begin_drag(
        &mut self,
        press: Press,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let ids = self.tabs.iter().map(|tab| tab.id).collect();
        let spans = self.slot_spans();
        let x = self.content_x(press.origin);
        let Some(engine) = StripDrag::begin(ids, spans, press.index, x) else {
            return;
        };
        let grab_y =
            f32::from(press.origin.y - self.row.origin.y) - strip_metrics::SCROLLER_PADDING;
        self.drag = Some(DragSession::new(engine, position, grab_y, self.tabs.len()));
        cx.notify();
    }

    fn settle_into_slot(
        &mut self,
        drag: &DragSession,
        cx: &Context<'_, Self>,
    ) {
        let Some(tab) = drag.engine.tab() else {
            return;
        };
        let x = self.content_x(drag.pointer);
        let slot = self
            .slot_spans()
            .get(drag.engine.target())
            .map_or(0.0, |span| span.x);
        let from = drag.engine.ghost_x(x) - slot;
        self.settle = Some(Settle::new(tab, from, self.clock.now(), Motion::of(cx)));
    }
}

fn dropped_out(
    drag: &DragSession,
    position: Point<Pixels>,
) -> Option<Event> {
    let tab = drag.engine.tab().filter(|_| drag.out)?;
    Some(Event::DropOut { tab, position })
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
    use layout::{Command, TabId};

    use crate::event::Event;
    use crate::test_support::{open, view};

    fn tabs() -> Vec<crate::pill::TabView> {
        vec![view(1, &["a"]), view(2, &["b"]), view(3, &["c"])]
    }

    fn applied(events: &[Event]) -> Vec<Vec<Command>> {
        events
            .iter()
            .filter_map(|event| match event {
                Event::Apply(commands) => Some(commands.clone()),
                _ => None,
            })
            .collect()
    }

    #[gpui::test]
    fn clicking_a_tab_activates_it(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (cx, events) = (harness.cx, harness.events);
        cx.simulate_click(point(px(8.0 + 240.0 + 120.0), px(21.0)), Modifiers::none());
        assert_eq!(
            applied(&events.borrow()),
            vec![vec![Command::ActivateTab { tab: TabId(2) }]],
            "tab two"
        );
    }

    #[gpui::test]
    fn dragging_a_tab_past_its_neighbour_reorders_on_release(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (cx, events) = (harness.cx, harness.events);
        let at = |x: f32| point(px(x), px(21.0));
        cx.simulate_mouse_down(at(8.0 + 100.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(at(8.0 + 160.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(at(8.0 + 330.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_up(at(8.0 + 330.0), MouseButton::Left, Modifiers::none());
        assert_eq!(
            applied(&events.borrow()),
            vec![vec![Command::MoveTab {
                tab: TabId(1),
                to_index: 1
            }]],
            "first tab moved to the second slot, and no activation"
        );
    }

    #[gpui::test]
    fn a_dragged_tab_keeps_following_the_pointer_below_the_bar(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (bar, cx) = (harness.bar, harness.cx);
        let none = Modifiers::none();
        cx.simulate_mouse_down(point(px(108.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(130.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(400.0), px(300.0)), MouseButton::Left, none);
        let pointer = bar.read_with(cx, |bar, _| bar.drag.as_ref().map(|drag| drag.pointer));
        assert_eq!(
            pointer,
            Some(point(px(400.0), px(300.0))),
            "the ghost follows the pointer out of the bar"
        );
    }

    fn left_the_strip(events: &[Event]) -> Vec<Event> {
        events
            .iter()
            .filter(|event| !matches!(event, Event::Apply(_)))
            .cloned()
            .collect()
    }

    #[gpui::test]
    fn a_tab_dragged_onto_the_panes_is_handed_out_and_dropped_there(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (cx, events) = (harness.cx, harness.events);
        let none = Modifiers::none();
        let below = point(px(400.0), px(300.0));
        cx.simulate_mouse_down(point(px(350.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(370.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(below, MouseButton::Left, none);
        cx.simulate_mouse_up(below, MouseButton::Left, none);
        let tab = TabId(2);
        let expected = vec![
            Event::DragOut {
                tab,
                position: below,
            },
            Event::DropOut {
                tab,
                position: below,
            },
        ];
        assert_eq!(left_the_strip(&events.borrow()), expected);
        assert!(applied(&events.borrow()).is_empty(), "no reorder");
    }

    #[gpui::test]
    fn a_tab_dragged_onto_the_panes_stays_faded_in_its_own_slot(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (bar, cx) = (harness.bar, harness.cx);
        let none = Modifiers::none();
        cx.simulate_mouse_down(point(px(108.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(130.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(600.0), px(300.0)), MouseButton::Left, none);
        let source = bar.read_with(cx, |bar, _| {
            let drag = bar.drag.as_ref()?;
            Some((drag.source_opacity(), drag.engine.target()))
        });
        assert_eq!(
            source,
            Some((0.4, 0)),
            "faded where it sat, with its neighbours back in place"
        );
    }

    #[gpui::test]
    fn a_tab_dragged_back_into_the_strip_cancels_its_drag_out(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (cx, events) = (harness.cx, harness.events);
        let none = Modifiers::none();
        let below = point(px(400.0), px(300.0));
        cx.simulate_mouse_down(point(px(350.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(370.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(below, MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(370.0), px(21.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(point(px(372.0), px(21.0)), MouseButton::Left, none);
        let expected = vec![
            Event::DragOut {
                tab: TabId(2),
                position: below,
            },
            Event::DragCancel,
        ];
        assert_eq!(left_the_strip(&events.borrow()), expected);
    }

    #[gpui::test]
    fn releasing_a_drag_where_it_started_writes_nothing(cx: &mut TestAppContext) {
        let harness = open(cx, tabs(), 1);
        let (cx, events) = (harness.cx, harness.events);
        let at = |x: f32| point(px(x), px(21.0));
        cx.simulate_mouse_down(at(108.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(at(130.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_move(at(108.0), MouseButton::Left, Modifiers::none());
        cx.simulate_mouse_up(at(108.0), MouseButton::Left, Modifiers::none());
        assert!(
            applied(&events.borrow()).is_empty(),
            "no command for a no-op drop"
        );
    }
}
