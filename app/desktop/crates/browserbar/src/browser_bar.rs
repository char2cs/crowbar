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

    pub fn begin_pick(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.picking = true;
        cx.notify();
    }

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

    use gpui::{TestAppContext, VisualTestContext};

    use super::*;

    const STEP: f32 = 6.0;
    const REACH: f32 = 1200.0;
    const BAND: f32 = 16.0;

    type Events = Rc<RefCell<Vec<Event>>>;

    fn open<'a>(
        address: &str,
        cx: &'a mut TestAppContext,
    ) -> (Entity<BrowserBar>, Events, &'a mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let (bar, cx) = cx.add_window_view(|_, cx| BrowserBar::new(cx));
        let events: Events = Rc::default();
        let sink = Rc::clone(&events);
        let push = move |_, event: &Event, _: &mut gpui::App| sink.borrow_mut().push(event.clone());
        cx.update(|_, cx| cx.subscribe(&bar, push).detach());
        bar.update(cx, |bar, cx| bar.set_address(address, cx));
        cx.run_until_parked();
        (bar, events, cx)
    }

    fn click_everywhere(cx: &mut VisualTestContext) {
        let middle = f32::from(cx.update(|window, _| window.viewport_size().height)) / 2.0;
        let along = |from: f32, to: f32| {
            std::iter::successors(Some(from), |at| Some(at + STEP)).take_while(move |at| *at < to)
        };
        let rows = along(middle - BAND, middle + BAND);
        let grid = rows.flat_map(|y| along(0.0, REACH).map(move |x| (x, y)));
        for (x, y) in grid {
            cx.simulate_click(gpui::point(px(x), px(y)), gpui::Modifiers::none());
        }
    }

    #[gpui::test]
    fn enter_in_the_field_asks_to_go_to_what_was_typed(cx: &mut TestAppContext) {
        let (bar, events, cx) = open("example.com", cx);
        let input = bar.read_with(cx, |bar, _| bar.input.clone());
        cx.update(|window, cx| input.update(cx, |field, cx| field.focus(window, cx)));
        cx.simulate_keystrokes("enter");
        assert_eq!(*events.borrow(), vec![Event::Go("example.com".to_owned())]);
    }

    #[gpui::test]
    fn with_no_page_or_the_blank_page_no_button_asks_for_anything(cx: &mut TestAppContext) {
        let (bar, events, cx) = open("about:blank", cx);
        let shown = bar.read_with(cx, |bar, cx| bar.input.read(cx).text().to_owned());
        assert_eq!(shown, "", "the field shows its placeholder");
        click_everywhere(cx);
        bar.update(cx, |bar, cx| bar.set_address("", cx));
        click_everywhere(cx);
        assert_eq!(
            *events.borrow(),
            Vec::new(),
            "back, forward, reload and pick are disabled"
        );
    }

    #[gpui::test]
    fn every_button_asks_for_its_own_event(cx: &mut TestAppContext) {
        let (_, events, cx) = open("example.com", cx);
        click_everywhere(cx);
        let seen = events.borrow();
        for expected in [
            Event::Back,
            Event::Forward,
            Event::Reload,
            Event::TogglePick,
        ] {
            assert!(seen.contains(&expected), "{expected:?} in {seen:?}");
        }
    }
}
