//! One selectable row of a menu.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, Hsla, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, hsla,
};

use crate::floating::menu::menu_item::MenuAction;
use crate::floating::menu::menu_item::MenuTone;
use crate::floating::menu::row_handlers::RowHandlers;
use crate::foundation::Appearance;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IconWeight;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;

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

fn hint(
    action: &MenuAction,
    palette: &Palette,
) -> Option<Div> {
    let keys = action.shortcut.clone()?;
    Some(
        div()
            .ml_auto()
            .pl(spacing::STEP_3)
            .text_size(text_size::XS)
            .text_color(palette.muted_foreground().opacity(SHORTCUT_OPACITY))
            .child(keys),
    )
}

fn trailing(
    action: &MenuAction,
    color: Hsla,
) -> Option<Div> {
    if action.submenu.is_some() {
        let icon = Icon::new(IconName::CaretRight)
            .size(spacing::STEP_3_5)
            .color(color);
        return Some(div().ml_auto().child(icon));
    }
    check(action, color)
}

fn check(
    action: &MenuAction,
    color: Hsla,
) -> Option<Div> {
    let icon = Icon::new(IconName::Check)
        .weight(IconWeight::Bold)
        .size(spacing::STEP_3_5)
        .color(color);
    action.checked.then(|| div().ml_auto().child(icon))
}

impl MenuRow {
    fn shown(&self) -> Pose {
        if self.action.disabled {
            return Pose::Off;
        }
        self.highlight
    }
}

impl RenderOnce for MenuRow {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let action = &self.action;
        let (fill, text) = colors(action, self.shown(), palette, theme.appearance());
        let (index, handlers) = (self.index, self.handlers);
        div()
            .id(("menu-row", index))
            .flex()
            .items_center()
            .gap(spacing::STEP_1_5)
            .px(spacing::STEP_1_5)
            .py(spacing::STEP_1)
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
            .children(trailing(action, text))
    }
}

opaque_debug!(MenuRow);

#[cfg(test)]
mod tests {
    use super::*;
    use crate::foundation::Theme;

    fn plain() -> MenuAction {
        MenuAction::new("x", |_, _| {})
    }

    fn dangerous() -> MenuAction {
        plain().destructive()
    }

    #[test]
    fn plain_rows_highlight_with_the_accent_colours() {
        let palette = Theme::default().palette(Surface::Window);
        let plain = plain();
        assert_eq!(
            colors(&plain, Pose::Off, palette, Appearance::Light).1,
            palette.foreground()
        );
        assert_eq!(
            colors(&plain, Pose::On, palette, Appearance::Light),
            (palette.accent(), palette.accent_foreground())
        );
    }

    #[test]
    fn destructive_rows_tint_more_strongly_in_dark_mode() {
        let palette = Theme::default().palette(Surface::Window);
        let danger = dangerous();
        assert_eq!(
            colors(&danger, Pose::Off, palette, Appearance::Light).1,
            palette.destructive()
        );
        let light = colors(&danger, Pose::On, palette, Appearance::Light).0;
        let dark = colors(&danger, Pose::On, palette, Appearance::Dark).0;
        assert!(dark.a > light.a);
    }
}
