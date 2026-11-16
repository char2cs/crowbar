use gpui::{
    AppContext as _, Context, Entity, EventEmitter, IntoElement, ParentElement, Render, Styled,
    Subscription, Window, div, px,
};
use ui::{Button, ButtonVariant, IconName, Surface, TextInput, TextInputEvent, TextInputStyle};

use crate::browser_bar_event::BrowserBarEvent;

const PLACEHOLDER: &str = "Search or enter address";
const BAR_PADDING: f32 = 6.0;
const BAR_GAP: f32 = 4.0;

/// Back, forward, reload and pick buttons beside an address field. It emits [`BrowserBarEvent`] and
/// changes nothing else; the shell calls [`BrowserBar::set_address`] as the page navigates.
pub struct BrowserBar {
    input: Entity<TextInput>,
    picking: bool,
    _input_events: Subscription,
}

impl std::fmt::Debug for BrowserBar {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("BrowserBar").finish_non_exhaustive()
    }
}

impl EventEmitter<BrowserBarEvent> for BrowserBar {}

impl BrowserBar {
    /// A bar with an empty address field.
    pub fn new(cx: &mut Context<'_, Self>) -> Self {
        let input = cx.new(|cx| {
            TextInput::new(PLACEHOLDER, cx)
                .on(Surface::Window)
                .style(TextInputStyle::Compact)
        });
        let subscription = cx.subscribe(&input, |_, input, event, cx| submitted(&input, event, cx));
        Self {
            input,
            picking: false,
            _input_events: subscription,
        }
    }

    /// Shows `address` in the field, as the page navigates.
    pub fn set_address(
        &mut self,
        address: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.input
            .update(cx, |input, cx| input.set_text(address, cx));
    }

    /// Lights the pick button: a pick is in progress.
    pub fn begin_pick(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.picking = true;
        cx.notify();
    }

    /// Puts the pick button out: the pick ended.
    pub fn end_pick(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.picking = false;
        cx.notify();
    }
}

fn submitted(
    input: &Entity<TextInput>,
    event: &TextInputEvent,
    cx: &mut Context<'_, BrowserBar>,
) {
    if matches!(event, TextInputEvent::Submitted) {
        let typed = input.read(cx).text().to_owned();
        cx.emit(BrowserBarEvent::Go(typed));
    }
}

impl Render for BrowserBar {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let step = |id: &'static str, label: &'static str, event: BrowserBarEvent| {
            Button::new(id, label)
                .variant(ButtonVariant::Ghost)
                .on(Surface::Window)
                .on_click(cx.listener(move |_, _, _, cx| cx.emit(event.clone())))
        };
        let mut pick_variant = ButtonVariant::Ghost;
        if self.picking {
            pick_variant = ButtonVariant::Secondary;
        }
        div()
            .flex()
            .flex_row()
            .items_center()
            .size_full()
            .px(px(BAR_PADDING))
            .gap(px(BAR_GAP))
            .child(step("browser-back", "Back", BrowserBarEvent::Back))
            .child(step("browser-forward", "Forward", BrowserBarEvent::Forward))
            .child(
                Button::icon_only("browser-reload", IconName::ArrowClockwise)
                    .on(Surface::Window)
                    .on_click(cx.listener(|_, _, _, cx| cx.emit(BrowserBarEvent::Reload))),
            )
            .child(
                Button::icon_only("browser-pick", IconName::Crosshair)
                    .variant(pick_variant)
                    .on(Surface::Window)
                    .on_click(cx.listener(|_, _, _, cx| cx.emit(BrowserBarEvent::TogglePick))),
            )
            .child(div().flex_1().child(self.input.clone()))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;

    fn record(
        bar: &Entity<BrowserBar>,
        events: &Rc<RefCell<Vec<BrowserBarEvent>>>,
        cx: &mut gpui::VisualTestContext,
    ) {
        let sink = Rc::clone(events);
        cx.update(|_, cx| cx.subscribe(bar, collect(sink)).detach());
    }

    fn collect(
        sink: Rc<RefCell<Vec<BrowserBarEvent>>>
    ) -> impl FnMut(Entity<BrowserBar>, &BrowserBarEvent, &mut gpui::App) + 'static {
        move |_, event, _| sink.borrow_mut().push(event.clone())
    }

    fn prepare(
        bar: &Entity<BrowserBar>,
        window: &mut Window,
        cx: &mut gpui::App,
    ) {
        bar.update(cx, |bar, cx| {
            bar.set_address("example.com", cx);
            bar.input.read(cx).focus(window);
        });
    }

    #[gpui::test]
    fn enter_in_the_field_asks_to_go_to_what_was_typed(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (bar, cx) = cx.add_window_view(|_, cx| BrowserBar::new(cx));
        let events: Rc<RefCell<Vec<BrowserBarEvent>>> = Rc::default();
        record(&bar, &events, cx);
        cx.update(|window, cx| prepare(&bar, window, cx));
        cx.simulate_keystrokes("enter");
        assert_eq!(
            *events.borrow(),
            vec![BrowserBarEvent::Go("example.com".to_owned())]
        );
    }

    #[gpui::test]
    fn every_button_asks_for_its_own_event_and_the_pick_button_lights_while_picking(
        cx: &mut TestAppContext
    ) {
        const STEP: f32 = 6.0;
        const REACH: f32 = 1200.0;
        const BAND: f32 = 16.0;
        cx.update(|cx| ui::init(cx).unwrap());
        let (bar, cx) = cx.add_window_view(|_, cx| BrowserBar::new(cx));
        let events: Rc<RefCell<Vec<BrowserBarEvent>>> = Rc::default();
        record(&bar, &events, cx);
        cx.run_until_parked();
        let middle = f32::from(cx.update(|window, _| window.viewport_size().height)) / 2.0;
        let along = |from: f32, to: f32| {
            std::iter::successors(Some(from), |at| Some(at + STEP)).take_while(move |at| *at < to)
        };
        let rows = along(middle - BAND, middle + BAND);
        let grid = rows.flat_map(|y| along(0.0, REACH).map(move |x| (x, y)));
        for (x, y) in grid {
            cx.simulate_click(gpui::point(px(x), px(y)), gpui::Modifiers::none());
        }
        let seen = events.borrow().clone();
        for expected in [
            BrowserBarEvent::Back,
            BrowserBarEvent::Forward,
            BrowserBarEvent::Reload,
            BrowserBarEvent::TogglePick,
        ] {
            assert!(seen.contains(&expected), "{expected:?} in {seen:?}");
        }
        bar.update(cx, BrowserBar::begin_pick);
        assert!(bar.read_with(cx, |bar, _| bar.picking));
        bar.update(cx, BrowserBar::end_pick);
        assert!(!bar.read_with(cx, |bar, _| bar.picking));
    }
}
