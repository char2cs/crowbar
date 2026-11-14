//! A keyboard-navigable list of actions in a popup frame.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Context, DismissEvent, EventEmitter, FocusHandle, Focusable, FontWeight,
    InteractiveElement, IntoElement, ParentElement, Pixels, Render, Styled, Window, canvas, div,
    px,
};

use crate::direction::Direction;
use crate::font::UI_FAMILY;
use crate::menu_actions::{CONTEXT, Confirm, Dismiss, SelectNext, SelectPrevious};
use crate::menu_cursor::step;
use crate::menu_item::MenuItem;
use crate::menu_row::MenuRow;
use crate::opaque_debug::opaque_debug;
use crate::overlay_guard::OverlayGuard;
use crate::overlays::Overlays;
use crate::palette::Palette;
use crate::pose::Pose;
use crate::radius;
use crate::row_handlers::RowHandlers;
use crate::shadow;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

/// The popup frame with its rows; emits [`DismissEvent`] when it should close.
pub struct Menu {
    items: Rc<[MenuItem]>,
    highlighted: Option<usize>,
    min_width: Pixels,
    focus_handle: FocusHandle,
    overlay: Rc<OverlayGuard>,
}

impl EventEmitter<DismissEvent> for Menu {}

impl Focusable for Menu {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Menu {
    pub(crate) const DEFAULT_MIN_WIDTH: Pixels = px(128.0);

    /// Builds a menu and takes focus so the arrow keys work at once.
    pub fn new(
        items: impl Into<Rc<[MenuItem]>>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle);
        Self {
            items: items.into(),
            highlighted: None,
            min_width: Self::DEFAULT_MIN_WIDTH,
            focus_handle,
            overlay: Rc::new(Overlays::open(cx, gpui::Bounds::default())),
        }
    }

    /// Widens the menu to at least `width`, as selects do to match their trigger.
    #[must_use]
    pub fn min_width(mut self, width: Pixels) -> Self {
        self.min_width = width;
        self
    }

    fn move_highlight(&mut self, direction: Direction, cx: &mut Context<'_, Self>) {
        self.highlighted = step(&self.items, self.highlighted, direction);
        cx.notify();
    }

    fn choose(&mut self, index: usize, window: &mut Window, cx: &mut Context<'_, Self>) {
        let Some(MenuItem::Action(action)) =
            self.items.get(index).filter(|item| item.is_selectable())
        else {
            return;
        };
        let on_select = Rc::clone(&action.on_select);
        cx.emit(DismissEvent);
        on_select(window, cx);
    }

    fn confirm(&mut self, _: &Confirm, window: &mut Window, cx: &mut Context<'_, Self>) {
        if let Some(index) = self.highlighted {
            self.choose(index, window, cx);
        }
    }

    fn highlight(&mut self, index: usize, cx: &mut Context<'_, Self>) {
        if self.highlighted != Some(index) {
            self.highlighted = Some(index);
            cx.notify();
        }
    }

    fn handlers(cx: &Context<'_, Self>) -> RowHandlers {
        let entity = cx.entity();
        let hover_entity = entity.clone();
        RowHandlers {
            hover: Rc::new(move |index, _, cx| {
                hover_entity.update(cx, |this, cx| this.highlight(index, cx));
            }),
            choose: Rc::new(move |index, window, cx| {
                entity.update(cx, |this, cx| this.choose(index, window, cx));
            }),
        }
    }

    fn entry(
        &self,
        (index, item): (usize, &MenuItem),
        palette: &Palette,
        handlers: &RowHandlers,
    ) -> AnyElement {
        match item {
            MenuItem::Action(action) => {
                let highlight = Pose::from(self.highlighted == Some(index));
                MenuRow::new(index, action.clone(), highlight, handlers.clone()).into_any_element()
            },
            MenuItem::Separator => div()
                .my(px(4.0))
                .mx(px(-4.0))
                .h(px(1.0))
                .bg(palette.border())
                .into_any_element(),
            MenuItem::Label(text) => div()
                .px(px(6.0))
                .py(px(4.0))
                .text_size(text_size::XS)
                .font_weight(FontWeight::MEDIUM)
                .text_color(palette.muted_foreground())
                .child(text.clone())
                .into_any_element(),
        }
    }
}

impl Render for Menu {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let guard = Rc::clone(&self.overlay);
        let handlers = Self::handlers(cx);
        let rows = self
            .items
            .iter()
            .enumerate()
            .map(|entry| self.entry(entry, palette, &handlers));
        div()
            .id("ui-menu")
            .debug_selector(|| "ui-menu".into())
            .key_context(CONTEXT)
            .track_focus(&self.focus_handle)
            .on_action(cx.listener(|this, _: &SelectNext, _, cx| {
                this.move_highlight(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &SelectPrevious, _, cx| {
                this.move_highlight(Direction::Backward, cx);
            }))
            .on_action(cx.listener(Self::confirm))
            .on_action(cx.listener(|_, _: &Dismiss, _, cx| cx.emit(DismissEvent)))
            .relative()
            .occlude()
            .flex()
            .flex_col()
            .min_w(self.min_width)
            .p(px(4.0))
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.foreground().opacity(0.1))
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(UI_FAMILY)
            .shadow(shadow::md(0.1))
            .child(
                canvas(move |bounds, _, _| guard.update(bounds), |_, (), _, _| {})
                    .absolute()
                    .size_full(),
            )
            .children(rows)
    }
}

opaque_debug!(Menu);

#[cfg(test)]
#[path = "menu_tests.rs"]
mod tests;
