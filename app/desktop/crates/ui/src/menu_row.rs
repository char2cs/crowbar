//! One selectable row of a menu.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, Hsla, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, hsla, px,
};

use crate::appearance::Appearance;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;
use crate::menu_action::MenuAction;
use crate::menu_tone::MenuTone;
use crate::opaque_debug::opaque_debug;
use crate::palette::Palette;
use crate::pose::Pose;
use crate::radius;
use crate::row_handlers::RowHandlers;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

const DISABLED_OPACITY: f32 = 0.5;
const DESTRUCTIVE_TINT_LIGHT: f32 = 0.1;
const DESTRUCTIVE_TINT_DARK: f32 = 0.2;
const SHORTCUT_OPACITY: f32 = 0.72;

#[derive(IntoElement)]
pub(crate) struct MenuRow {
    index: usize,
    action: MenuAction,
    highlight: Pose,
    handlers: RowHandlers,
}

impl MenuRow {
    pub(crate) fn new(
        index: usize,
        action: MenuAction,
        highlight: Pose,
        handlers: RowHandlers,
    ) -> Self {
        Self {
            index,
            action,
            highlight,
            handlers,
        }
    }
}

fn colors(
    action: &MenuAction,
    highlight: Pose,
    palette: &Palette,
    appearance: Appearance,
) -> (Hsla, Hsla) {
    let clear = hsla(0.0, 0.0, 0.0, 0.0);
    match (action.tone, highlight) {
        (MenuTone::Destructive, Pose::On) => {
            let tint = match appearance {
                Appearance::Dark => DESTRUCTIVE_TINT_DARK,
                Appearance::Light => DESTRUCTIVE_TINT_LIGHT,
            };
            (palette.destructive().opacity(tint), palette.destructive())
        },
        (MenuTone::Destructive, Pose::Off) => (clear, palette.destructive()),
        (MenuTone::Normal, Pose::On) => (palette.accent(), palette.accent_foreground()),
        (MenuTone::Normal, Pose::Off) => (clear, palette.foreground()),
    }
}

fn hint(action: &MenuAction, palette: &Palette) -> Option<Div> {
    let keys = action.shortcut.clone()?;
    Some(
        div()
            .ml_auto()
            .pl(px(12.0))
            .text_size(text_size::XS)
            .text_color(palette.muted_foreground().opacity(SHORTCUT_OPACITY))
            .child(keys),
    )
}

fn check(action: &MenuAction, color: Hsla) -> Option<Div> {
    let icon = Icon::new(IconName::Check)
        .weight(IconWeight::Bold)
        .size(px(14.0))
        .color(color);
    action.checked.then(|| div().ml_auto().child(icon))
}

impl RenderOnce for MenuRow {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let action = &self.action;
        let highlight = if action.disabled {
            Pose::Off
        } else {
            self.highlight
        };
        let (fill, text) = colors(action, highlight, palette, theme.appearance());
        let (index, handlers) = (self.index, self.handlers);
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
                    .on_mouse_move(move |_, window, cx| (handlers.hover)(index, window, cx))
                    .on_click(move |_, window, cx| (handlers.choose)(index, window, cx))
            })
            .children(action.icon.map(|name| Icon::new(name).color(text)))
            .child(action.label.clone())
            .children(hint(action, palette))
            .children(check(action, text))
    }
}

opaque_debug!(MenuRow);

#[cfg(test)]
#[path = "menu_row_tests.rs"]
mod tests;
