use gpui::{
    AppContext as _, Context, Entity, EventEmitter, InteractiveElement, IntoElement, KeyDownEvent,
    ParentElement, Render, StatefulInteractiveElement, Styled, Subscription, Window, div, px,
};
use ui::{Dialog, DialogSize, IconName, Surface, TextInput, TextInputEvent, TextInputStyle, Theme};

use crate::command_palette::rank;
use crate::palette_entry::PaletteEntry;
use crate::palette_event::PaletteEvent;
use crate::palette_row::row;
use crate::ranked_entry::RankedEntry;
use crate::row_state::RowState;
use crate::step_direction::StepDirection;

const LIST_HEIGHT: f32 = 320.0;
const BODY_GAP: f32 = 8.0;
const PLACEHOLDER: &str = "Type a command";

/// The palette dialog: a search field over the ranked commands. Arrow keys move the highlight,
/// `Enter` runs it and `Escape` closes; it emits [`PaletteEvent`] and changes nothing else.
pub struct PalettePanel {
    entries: Vec<PaletteEntry>,
    ranked: Vec<RankedEntry>,
    selected: usize,
    input: Entity<TextInput>,
    _input_events: Subscription,
}

impl std::fmt::Debug for PalettePanel {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("PalettePanel")
            .field("entries", &self.entries.len())
            .finish_non_exhaustive()
    }
}

impl EventEmitter<PaletteEvent> for PalettePanel {}

impl PalettePanel {
    /// A palette over `entries`, with the search field focused.
    pub fn new(
        entries: Vec<PaletteEntry>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let input = cx.new(|cx| {
            TextInput::new(PLACEHOLDER, cx)
                .on(Surface::Window)
                .style(TextInputStyle::Standard)
                .leading_icon(IconName::MagnifyingGlass)
        });
        input.read(cx).focus(window);
        let subscription = cx.subscribe(&input, Self::on_input);
        Self {
            ranked: rank(&entries, ""),
            entries,
            selected: 0,
            input,
            _input_events: subscription,
        }
    }

    fn on_input(
        &mut self,
        _: Entity<TextInput>,
        event: &TextInputEvent,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            TextInputEvent::Changed(query) => self.requery(query, cx),
            TextInputEvent::Submitted => self.run(self.selected, cx),
            TextInputEvent::Cancelled => cx.emit(PaletteEvent::Close),
            _ => {},
        }
    }

    fn requery(
        &mut self,
        query: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.ranked = rank(&self.entries, query);
        self.selected = 0;
        cx.notify();
    }

    fn run(
        &mut self,
        index: usize,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(hit) = self.ranked.get(index) {
            cx.emit(PaletteEvent::Run(hit.entry.id.clone()));
        }
    }

    fn on_key(
        &mut self,
        event: &KeyDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(direction) = StepDirection::from_key(event.keystroke.key.as_str()) else {
            return;
        };
        cx.stop_propagation();
        self.selected = direction.apply(self.selected, self.ranked.len());
        cx.notify();
    }
}

impl Render for PalettePanel {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let rows = self.ranked.iter().enumerate().map(|(index, hit)| {
            row(hit, RowState::at(index, self.selected), *palette)
                .id(("palette-row", index))
                .cursor_pointer()
                .on_click(cx.listener(move |this, _, _, cx| this.run(index, cx)))
        });
        let list = div()
            .id("palette-list")
            .flex()
            .flex_col()
            .max_h(px(LIST_HEIGHT))
            .overflow_y_scroll()
            .children(rows);
        let me = cx.entity();
        let body = div()
            .flex()
            .flex_col()
            .gap(px(BODY_GAP))
            .capture_key_down(cx.listener(|this, event, _, cx| this.on_key(event, cx)))
            .child(self.input.clone())
            .child(list);
        Dialog::new("command-palette")
            .size(DialogSize::Md)
            .body(body)
            .on_close(move |_, cx| me.update(cx, |_, cx| cx.emit(PaletteEvent::Close)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{TestAppContext, VisualTestContext};

    use super::*;

    fn entry(
        id: &str,
        title: &str,
    ) -> PaletteEntry {
        PaletteEntry {
            id: id.to_owned(),
            title: title.to_owned(),
            shortcut: None,
        }
    }

    fn record(
        panel: &Entity<PalettePanel>,
        events: &Rc<RefCell<Vec<PaletteEvent>>>,
        cx: &mut VisualTestContext,
    ) {
        let sink = Rc::clone(events);
        cx.update(|_, cx| cx.subscribe(panel, collect(sink)).detach());
    }

    fn collect(
        sink: Rc<RefCell<Vec<PaletteEvent>>>
    ) -> impl FnMut(Entity<PalettePanel>, &PaletteEvent, &mut gpui::App) + 'static {
        move |_, event, _| sink.borrow_mut().push(event.clone())
    }

    fn open(
        cx: &mut TestAppContext,
        events: &Rc<RefCell<Vec<PaletteEvent>>>,
    ) -> &mut VisualTestContext {
        cx.update(|cx| ui::init(cx).unwrap());
        let entries = vec![entry("a", "Zoom"), entry("b", "New tab")];
        let (panel, cx) = cx.add_window_view(|window, cx| PalettePanel::new(entries, window, cx));
        record(&panel, events, cx);
        cx.run_until_parked();
        cx
    }

    #[gpui::test]
    fn the_arrow_keys_move_the_highlight_and_enter_runs_it(cx: &mut TestAppContext) {
        let events = Rc::default();
        let cx = open(cx, &events);
        cx.simulate_keystrokes("down enter");
        assert_eq!(*events.borrow(), vec![PaletteEvent::Run("b".to_owned())]);
    }

    #[gpui::test]
    fn escape_closes_the_palette(cx: &mut TestAppContext) {
        let events = Rc::default();
        let cx = open(cx, &events);
        cx.simulate_keystrokes("escape");
        assert_eq!(*events.borrow(), vec![PaletteEvent::Close]);
    }
}
