use gpui::{App, Context, Div, KeyDownEvent, ParentElement, SharedString, Styled, div, px};
use ui::{Button, ButtonSize, ButtonVariant, IconName, Surface, Theme, Tooltip};

use crate::search_bar::toggle_spec::ToggleSpec;
use crate::search_bar::{Event, SearchBar};

const ROW_GAP: f32 = 6.0;
const BUTTON_GAP: f32 = 4.0;
const CLEAR_INSET: f32 = 4.0;

impl SearchBar {
    pub(super) fn counter(
        &self,
        cx: &App,
    ) -> Option<SharedString> {
        if self.input.read(cx).text().is_empty() {
            return None;
        }
        Some(format!("{}/{}", self.summary.current, self.summary.total).into())
    }

    fn can_navigate(
        &self,
        cx: &App,
    ) -> bool {
        !self.input.read(cx).text().is_empty() && self.summary.total > 0
    }

    fn clear(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.input.update(cx, |input, cx| input.set_text("", cx));
        self.emit_query(cx);
        cx.notify();
    }

    fn toggle_button(
        &self,
        spec: ToggleSpec,
        cx: &mut Context<'_, Self>,
    ) -> Tooltip {
        let ToggleSpec {
            id,
            label,
            icon,
            option,
        } = spec;
        let pressed = self
            .options
            .contains(option)
            .then_some(ButtonVariant::Outline);
        let variant = pressed.unwrap_or(ButtonVariant::Ghost);
        let me = cx.entity();
        let button = Button::icon_only(id, icon)
            .variant(variant)
            .size(ButtonSize::Xs)
            .on_click(move |_, _, app| me.update(app, |bar, cx| bar.toggle(option, cx)));
        Tooltip::new(SharedString::from(format!("{id}-tip")), label, button)
    }

    fn event_button(
        (id, icon): (&'static str, IconName),
        event: Event,
        cx: &mut Context<'_, Self>,
    ) -> Button {
        let me = cx.entity();
        Button::icon_only(id, icon)
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Xs)
            .on_click(move |_, _, app| me.update(app, |_, cx| cx.emit(event.clone())))
    }

    pub(super) fn search_row(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let muted = Theme::of(cx).palette(Surface::Window).muted_foreground();
        let counter = self.counter(cx).map(|count| {
            div()
                .flex_none()
                .text_size(ui::text_size::SM)
                .text_color(muted)
                .child(count)
        });
        let close = Self::event_button(("search-close", IconName::X), Event::Close, cx);
        div()
            .flex()
            .items_center()
            .gap(px(ROW_GAP))
            .child(self.query_field(cx))
            .children(counter)
            .child(close)
    }

    fn query_field(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let me = cx.entity();
        let clear = (!self.input.read(cx).text().is_empty()).then(|| {
            let button = Button::icon_only("search-clear", IconName::X)
                .variant(ButtonVariant::Ghost)
                .size(ButtonSize::Xs)
                .on_click(move |_, _, app| me.update(app, SearchBar::clear));
            div()
                .absolute()
                .top_0()
                .bottom_0()
                .right(px(CLEAR_INSET))
                .flex()
                .items_center()
                .child(button)
        });
        div()
            .relative()
            .flex_1()
            .min_w_0()
            .child(self.input.clone())
            .children(clear)
    }

    pub(super) fn option_row(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let toggles = div()
            .flex()
            .items_center()
            .gap(px(BUTTON_GAP))
            .child(self.toggle_button(ToggleSpec::CASE_SENSITIVE, cx))
            .child(self.toggle_button(ToggleSpec::WHOLE_WORD, cx))
            .child(self.toggle_button(ToggleSpec::REGEX, cx));
        let steps = self.steps(cx);
        div()
            .mt(px(ROW_GAP))
            .flex()
            .items_center()
            .justify_between()
            .gap(px(BUTTON_GAP * 2.0))
            .child(toggles)
            .child(steps)
    }

    fn steps(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let previous =
            Self::event_button(("search-previous", IconName::CaretUp), Event::Previous, cx);
        let next = Self::event_button(("search-next", IconName::CaretDown), Event::Next, cx);
        let row = div().flex().items_center().gap(px(BUTTON_GAP));
        if self.can_navigate(cx) {
            return row.child(previous).child(next);
        }
        row.child(previous.disabled()).child(next.disabled())
    }

    pub(super) fn on_card_key(
        event: &KeyDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let keystroke = &event.keystroke;
        if keystroke.key != "enter" || !keystroke.modifiers.shift {
            return;
        }
        cx.stop_propagation();
        cx.emit(Event::Previous);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use terminal::{SearchOptions, SearchQuery, SearchSummary};

    use super::*;
    use crate::test_support::{open, record_events};

    #[gpui::test]
    fn steps_need_a_match_and_clearing_empties_the_query(cx: &mut TestAppContext) {
        let (_, cx) = open(cx);
        let (bar, cx) = cx.add_window_view(SearchBar::new);
        let queries = record_events::<SearchBar, Event>(cx, &bar);
        bar.update(cx, |bar, cx| {
            bar.input.update(cx, |input, cx| input.set_text("cat", cx));
        });
        assert!(!bar.read_with(cx, SearchBar::can_navigate), "no match yet");
        let summary = SearchSummary {
            current: 0,
            total: 0,
        };
        bar.update(cx, |bar, cx| bar.show_summary(summary, cx));
        assert_eq!(bar.read_with(cx, SearchBar::counter), Some("0/0".into()));
        let found = SearchSummary {
            current: 1,
            total: 3,
        };
        bar.update(cx, |bar, cx| bar.show_summary(found, cx));
        assert!(bar.read_with(cx, SearchBar::can_navigate));
        bar.update(cx, SearchBar::clear);
        let last = queries.borrow().last().cloned();
        let empty = SearchQuery {
            text: String::new(),
            options: SearchOptions::NONE,
        };
        assert_eq!(last, Some(Event::QueryChanged(empty)));
    }
}
