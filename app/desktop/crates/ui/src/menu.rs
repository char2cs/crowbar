//! A keyboard-navigable list of actions in a popup frame.

use std::rc::Rc;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Context, DismissEvent, EventEmitter, FocusHandle, Focusable, FontWeight, InteractiveElement, IntoElement,
    ParentElement, Pixels, Render, StatefulInteractiveElement, Styled, Window, canvas, div, px,
};

use crate::appearance::Appearance;
use crate::font::UI_FAMILY;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;
use crate::menu_action::MenuAction;
use crate::menu_actions::{CONTEXT, Confirm, Dismiss, SelectNext, SelectPrevious};
use crate::menu_cursor::step;
use crate::menu_item::MenuItem;
use crate::overlay_guard::OverlayGuard;
use crate::overlays::Overlays;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

const MIN_WIDTH: f32 = 128.0;
const DISABLED_OPACITY: f32 = 0.5;

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
    /// Builds a menu and takes focus so the arrow keys work at once.
    pub fn new(items: impl Into<Rc<[MenuItem]>>, window: &mut Window, cx: &mut Context<Self>) -> Self {
        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle);
        Self {
            items: items.into(),
            highlighted: None,
            min_width: px(MIN_WIDTH),
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

    fn move_highlight(&mut self, forward: bool, cx: &mut Context<Self>) {
        self.highlighted = step(&self.items, self.highlighted, forward);
        cx.notify();
    }

    fn choose(&mut self, index: usize, window: &mut Window, cx: &mut Context<Self>) {
        let Some(MenuItem::Action(action)) = self.items.get(index).filter(|item| item.is_selectable()) else {
            return;
        };
        let on_select = Rc::clone(&action.on_select);
        cx.emit(DismissEvent);
        on_select(window, cx);
    }

    fn confirm(&mut self, _: &Confirm, window: &mut Window, cx: &mut Context<Self>) {
        if let Some(index) = self.highlighted {
            self.choose(index, window, cx);
        }
    }

    fn row(&self, index: usize, action: &MenuAction, palette: &Palette, appearance: Appearance, cx: &mut Context<Self>) -> impl IntoElement {
        let highlighted = self.highlighted == Some(index) && !action.disabled;
        let (fill, text) = highlight(action, highlighted, palette, appearance);
        let icon = action.icon.map(|name| Icon::new(name).color(text));
        let check = action.checked.then(|| Icon::new(IconName::Check).weight(IconWeight::Bold).size(px(14.0)).color(text));
        div()
            .id(("menu-row", index))
            .flex()
            .items_center()
            .gap(px(6.0))
            .px(px(6.0))
            .py(px(4.0))
            .rounded(radius::MD)
            .bg(fill)
            .text_color(text)
            .text_size(text_size::SM)
            .when(action.disabled, |d| d.opacity(DISABLED_OPACITY))
            .when(!action.disabled, |d| {
                d.cursor_pointer()
                    .on_mouse_move(cx.listener(move |this, _, _, cx| {
                        if this.highlighted != Some(index) {
                            this.highlighted = Some(index);
                            cx.notify();
                        }
                    }))
                    .on_click(cx.listener(move |this, _, window, cx| this.choose(index, window, cx)))
            })
            .children(icon)
            .child(action.label.clone())
            .when_some(action.shortcut.clone(), |d, keys| {
                d.child(div().ml_auto().pl(px(12.0)).text_size(text_size::XS).text_color(palette.muted_foreground().opacity(0.72)).child(keys))
            })
            .children(check.map(|check| div().ml_auto().child(check)))
    }
}

fn highlight(action: &MenuAction, highlighted: bool, palette: &Palette, appearance: Appearance) -> (gpui::Hsla, gpui::Hsla) {
    let clear = gpui::hsla(0.0, 0.0, 0.0, 0.0);
    match (action.destructive, highlighted) {
        (true, true) => {
            let tint = if appearance == Appearance::Dark { 0.2 } else { 0.1 };
            (palette.destructive().opacity(tint), palette.destructive())
        }
        (true, false) => (clear, palette.destructive()),
        (false, true) => (palette.accent(), palette.accent_foreground()),
        (false, false) => (clear, palette.foreground()),
    }
}

impl Render for Menu {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let guard = Rc::clone(&self.overlay);
        let rows: Vec<_> = self
            .items
            .iter()
            .enumerate()
            .map(|(index, item)| match item {
                MenuItem::Action(action) => self.row(index, action, palette, theme.appearance(), cx).into_any_element(),
                MenuItem::Separator => div().my(px(4.0)).mx(px(-4.0)).h(px(1.0)).bg(palette.border()).into_any_element(),
                MenuItem::Label(text) => div()
                    .px(px(6.0))
                    .py(px(4.0))
                    .text_size(text_size::XS)
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(palette.muted_foreground())
                    .child(text.clone())
                    .into_any_element(),
            })
            .collect();
        div()
            .id("ui-menu")
            .key_context(CONTEXT)
            .track_focus(&self.focus_handle)
            .on_action(cx.listener(|this, _: &SelectNext, _, cx| this.move_highlight(true, cx)))
            .on_action(cx.listener(|this, _: &SelectPrevious, _, cx| this.move_highlight(false, cx)))
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
            .child(canvas(move |bounds, _, _| guard.update(bounds), |_, (), _, _| {}).absolute().size_full())
            .children(rows)
    }
}

#[cfg(test)]
#[path = "menu_tests.rs"]
mod tests;
