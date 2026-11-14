//! The list of entries of the context menu, nested lists included.

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ClickEvent, Div, Entity, FocusHandle, FontWeight, InteractiveElement,
    IntoElement, MouseMoveEvent, ParentElement, RenderOnce, Stateful, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use ui::font::UI_FAMILY;
use ui::{Icon, IconName, Palette, Surface, Theme, radius, shadow, text_size};

use crate::intent::Intent;
use crate::item_look::ItemLook;
use crate::menu_entry::MenuEntry;
use crate::sidebar::Sidebar;
use crate::tone::Tone;

const MIN_WIDTH: f32 = 180.0;
const ROW_MIN_HEIGHT: f32 = 28.0;
const ICON_ALPHA: f32 = 0.8;

#[derive(IntoElement)]
pub(crate) struct MenuList {
    pub(crate) sidebar: Entity<Sidebar>,
    pub(crate) entries: Vec<MenuEntry>,
    pub(crate) highlighted: Option<usize>,
    pub(crate) submenu: Option<usize>,
    pub(crate) focus: Option<FocusHandle>,
}

impl RenderOnce for MenuList {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let rows: Vec<AnyElement> = self
            .entries
            .iter()
            .enumerate()
            .map(|(index, entry)| self.row(index, entry, palette))
            .collect();
        let sidebar = self.sidebar.clone();
        div()
            .id("sidebar-menu")
            .when_some(self.focus.clone(), |d, focus| d.track_focus(&focus))
            .on_key_down(move |event, window, cx| {
                let key = event.keystroke.key.clone();
                sidebar.update(cx, |this, cx| this.menu_key(&key, window, cx));
            })
            .occlude()
            .relative()
            .flex()
            .flex_col()
            .min_w(px(MIN_WIDTH))
            .p(px(4.0))
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.border())
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(UI_FAMILY)
            .text_size(text_size::SM)
            .shadow(shadow::lg(0.05))
            .children(rows)
    }
}

impl MenuList {
    fn row(&self, index: usize, entry: &MenuEntry, palette: &'static Palette) -> AnyElement {
        let active = self.highlighted == Some(index);
        match entry {
            MenuEntry::Separator => separator(palette).into_any_element(),
            MenuEntry::Action {
                label,
                icon,
                tone,
                intent,
            } => {
                let look = ItemLook {
                    icon: *icon,
                    tone: *tone,
                    active,
                    palette,
                };
                item(index, label, look)
                    .on_mouse_move(highlighter(&self.sidebar, index))
                    .on_click(chooser(&self.sidebar, intent.clone()))
                    .into_any_element()
            },
            MenuEntry::Submenu {
                label,
                icon,
                entries,
            } => {
                let look = ItemLook {
                    icon: *icon,
                    tone: Tone::Normal,
                    active,
                    palette,
                };
                let nested = (self.submenu == Some(index)).then(|| self.nested(entries));
                item(index, label, look)
                    .on_mouse_move(highlighter(&self.sidebar, index))
                    .relative()
                    .child(caret(palette))
                    .children(nested)
                    .into_any_element()
            },
        }
    }

    fn nested(&self, entries: &[MenuEntry]) -> Div {
        let list = MenuList {
            sidebar: self.sidebar.clone(),
            entries: entries.to_vec(),
            highlighted: None,
            submenu: None,
            focus: None,
        };
        div().absolute().left_full().top(px(-4.0)).child(list)
    }
}

fn separator(palette: &Palette) -> Div {
    div()
        .mx(px(8.0))
        .my(px(4.0))
        .h(px(1.0))
        .bg(palette.border())
}

fn caret(palette: &Palette) -> Div {
    let icon = Icon::new(IconName::CaretRight)
        .size(px(16.0))
        .color(palette.foreground().opacity(0.6));
    div().ml_auto().child(icon)
}

fn highlighter(
    sidebar: &Entity<Sidebar>,
    index: usize,
) -> impl Fn(&MouseMoveEvent, &mut Window, &mut App) + 'static {
    let sidebar = sidebar.clone();
    move |_, _, cx| sidebar.update(cx, |this, cx| this.highlight(Some(index), cx))
}

fn chooser(
    sidebar: &Entity<Sidebar>,
    intent: Intent,
) -> impl Fn(&ClickEvent, &mut Window, &mut App) + 'static {
    let sidebar = sidebar.clone();
    move |_, window, cx| {
        sidebar.update(cx, |this, cx| this.choose(intent.clone(), window, cx));
    }
}

fn item(index: usize, label: &str, look: ItemLook) -> Stateful<Div> {
    let palette = look.palette;
    let (fill, text) = match (look.tone, look.active) {
        (Tone::Destructive, true) => (palette.destructive().opacity(0.1), palette.destructive()),
        (Tone::Destructive, false) => (gpui::transparent_black(), palette.destructive()),
        (Tone::Normal, true) => (palette.accent(), palette.accent_foreground()),
        (Tone::Normal, false) => (gpui::transparent_black(), palette.foreground()),
    };
    let icon = look.icon.map(|name| {
        let glyph = Icon::new(name).size(px(16.0)).color(text);
        div().mx(px(-2.0)).opacity(ICON_ALPHA).child(glyph)
    });
    div()
        .id(("sidebar-menu-item", index))
        .flex()
        .items_center()
        .gap(px(8.0))
        .min_h(px(ROW_MIN_HEIGHT))
        .px(px(8.0))
        .py(px(4.0))
        .rounded(radius::SM)
        .bg(fill)
        .text_color(text)
        .font_weight(FontWeight::NORMAL)
        .cursor_pointer()
        .children(icon)
        .child(label.to_owned())
}

#[cfg(test)]
#[path = "menu_list_tests.rs"]
mod tests;
