use gpui::prelude::FluentBuilder as _;
use gpui::{
    AppContext as _, Context, Entity, EventEmitter, IntoElement, ParentElement, Render, Styled,
    Subscription, Window, div, px,
};
use ui::{
    Button, ButtonSize, ButtonVariant, IconName, Surface, TextInput, TextInputEvent,
    TextInputStyle, Theme, Tooltip,
};

use crate::event::Event;

const PLACEHOLDER: &str = "Search or enter address";
const PICK_TIP: &str = "Pick element (copy pointer)";
const BLANK_PAGE: &str = "about:blank";
const BAR_PADDING: f32 = 8.0;
const BAR_GAP: f32 = 4.0;

/// Back, forward, reload and pick buttons beside an address field. It emits [`Event`] and
/// changes nothing else; the shell calls [`BrowserBar::set_address`] as the page navigates.
pub struct BrowserBar {
    input: Entity<TextInput>,
    picking: bool,
    has_page: bool,
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

impl EventEmitter<Event> for BrowserBar {}

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
            has_page: false,
            _input_events: subscription,
        }
    }

    /// Shows `address` in the field, as the page navigates. With none, or the blank page a
    /// browser opens on, there is no page to go back, forward, reload or pick in, so those
    /// buttons are disabled.
    pub fn set_address(
        &mut self,
        address: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let address = match address {
            BLANK_PAGE => "",
            page => page,
        };
        self.has_page = !address.is_empty();
        cx.notify();
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
        cx.emit(Event::Go(typed));
    }
}

impl Render for BrowserBar {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let has_page = self.has_page;
        let step = |id: &'static str, icon: IconName, event: Event| {
            Button::icon_only(id, icon)
                .size(ButtonSize::Sm)
                .on(Surface::Window)
                .on_click(cx.listener(move |_, _, _, cx| cx.emit(event.clone())))
                .when(!has_page, Button::disabled)
        };
        let mut pick = step("browser-pick", IconName::Crosshair, Event::TogglePick);
        if self.picking {
            pick = pick.variant(ButtonVariant::Secondary);
        }
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .flex()
            .flex_row()
            .items_center()
            .size_full()
            .px(px(BAR_PADDING))
            .gap(px(BAR_GAP))
            .border_b_1()
            .border_color(palette.border())
            .bg(palette.background())
            .child(step("browser-back", IconName::ArrowLeft, Event::Back))
            .child(step(
                "browser-forward",
                IconName::ArrowRight,
                Event::Forward,
            ))
            .child(step(
                "browser-reload",
                IconName::ArrowClockwise,
                Event::Reload,
            ))
            .child(Tooltip::new("browser-pick-tip", PICK_TIP, pick))
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
        events: &Rc<RefCell<Vec<Event>>>,
        cx: &mut gpui::VisualTestContext,
    ) {
        let sink = Rc::clone(events);
        cx.update(|_, cx| cx.subscribe(bar, collect(sink)).detach());
    }

    fn collect(
        sink: Rc<RefCell<Vec<Event>>>
    ) -> impl FnMut(Entity<BrowserBar>, &Event, &mut gpui::App) + 'static {
        move |_, event, _| sink.borrow_mut().push(event.clone())
    }

    fn prepare(
        bar: &Entity<BrowserBar>,
        window: &mut Window,
        cx: &mut gpui::App,
    ) {
        bar.update(cx, |bar, cx| {
            bar.set_address("example.com", cx);
            bar.input.update(cx, |field, cx| field.focus(window, cx));
        });
    }

    #[gpui::test]
    fn enter_in_the_field_asks_to_go_to_what_was_typed(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (bar, cx) = cx.add_window_view(|_, cx| BrowserBar::new(cx));
        let events: Rc<RefCell<Vec<Event>>> = Rc::default();
        record(&bar, &events, cx);
        cx.update(|window, cx| prepare(&bar, window, cx));
        cx.simulate_keystrokes("enter");
        assert_eq!(*events.borrow(), vec![Event::Go("example.com".to_owned())]);
    }

    const STEP: f32 = 6.0;
    const REACH: f32 = 1200.0;
    const BAND: f32 = 16.0;

    fn clicked_everywhere<'a>(
        address: &str,
        cx: &'a mut TestAppContext,
    ) -> (
        Entity<BrowserBar>,
        Vec<Event>,
        &'a mut gpui::VisualTestContext,
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (bar, cx) = cx.add_window_view(|_, cx| BrowserBar::new(cx));
        let events: Rc<RefCell<Vec<Event>>> = Rc::default();
        record(&bar, &events, cx);
        bar.update(cx, |bar, cx| bar.set_address(address, cx));
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
        (bar, seen, cx)
    }

    #[gpui::test]
    fn with_no_page_no_button_asks_for_anything(cx: &mut TestAppContext) {
        let (_, seen, _) = clicked_everywhere("", cx);
        assert_eq!(
            seen,
            Vec::new(),
            "back, forward, reload and pick are disabled"
        );
    }

    #[gpui::test]
    fn the_blank_page_a_browser_opens_on_is_no_page(cx: &mut TestAppContext) {
        let (bar, seen, cx) = clicked_everywhere("about:blank", cx);
        assert_eq!(seen, Vec::new(), "nothing to go back to, reload or pick in");
        let shown = bar.read_with(cx, |bar, cx| bar.input.read(cx).text().to_owned());
        assert_eq!(shown, "", "the field shows its placeholder");
    }

    #[gpui::test]
    fn every_button_asks_for_its_own_event_and_the_pick_button_lights_while_picking(
        cx: &mut TestAppContext
    ) {
        let (bar, seen, cx) = clicked_everywhere("example.com", cx);
        for expected in [
            Event::Back,
            Event::Forward,
            Event::Reload,
            Event::TogglePick,
        ] {
            assert!(seen.contains(&expected), "{expected:?} in {seen:?}");
        }
        bar.update(cx, BrowserBar::begin_pick);
        assert!(bar.read_with(cx, |bar, _| bar.picking));
        bar.update(cx, BrowserBar::end_pick);
        assert!(!bar.read_with(cx, |bar, _| bar.picking));
    }
}
