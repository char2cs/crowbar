//! The list of entries of the context menu, nested lists included.

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ClickEvent, Div, Entity, FocusHandle, FontWeight, InteractiveElement,
    IntoElement, MouseMoveEvent, ParentElement, Pixels, RenderOnce, Stateful,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, Palette, Surface, Theme, radius, shadow, text_size};

use crate::check::Check;
use crate::intent::Intent;
use crate::item_look::ItemLook;
use crate::menu_entry::MenuEntry;
use crate::scale::{ICON_LARGE, ICON_MEDIUM, SPACE_MD, SPACE_XS};
use crate::sidebar::Sidebar;
use crate::tone::Tone;

const SHADOW_STRENGTH: f32 = 0.05;
const SUBMENU_RISE: Pixels = px(-4.0);
const CARET_ALPHA: f32 = 0.6;
const DESTRUCTIVE_TINT: f32 = 0.1;
const ICON_PULL: Pixels = px(-2.0);

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
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let family = Theme::of(cx).ui_family();
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
            .p(SPACE_XS)
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.border())
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(family)
            .text_size(text_size::SM)
            .shadow(shadow::lg(SHADOW_STRENGTH))
            .children(rows)
    }
}

impl MenuList {
    fn row(
        &self,
        index: usize,
        entry: &MenuEntry,
        palette: &'static Palette,
    ) -> AnyElement {
        let active = self.highlighted == Some(index);
        match entry {
            MenuEntry::Separator => separator(palette).into_any_element(),
            MenuEntry::Action {
                label,
                icon,
                tone,
                intent,
                checked,
            } => self.action_row(
                index,
                (label, intent),
                ItemLook {
                    icon: *icon,
                    tone: *tone,
                    active,
                    checked: *checked,
                    palette,
                },
            ),
            MenuEntry::Submenu {
                label,
                icon,
                entries,
            } => self.submenu_row(
                index,
                (label, entries),
                ItemLook {
                    icon: *icon,
                    tone: Tone::Normal,
                    active,
                    checked: None,
                    palette,
                },
            ),
        }
    }

    fn action_row(
        &self,
        index: usize,
        (label, intent): (&str, &Intent),
        look: ItemLook,
    ) -> AnyElement {
        item(index, label, look)
            .on_mouse_move(highlighter(&self.sidebar, index))
            .on_click(chooser(&self.sidebar, intent.clone()))
            .into_any_element()
    }

    fn submenu_row(
        &self,
        index: usize,
        (label, entries): (&str, &[MenuEntry]),
        look: ItemLook,
    ) -> AnyElement {
        let palette = look.palette;
        let nested = (self.submenu == Some(index)).then(|| self.nested(entries));
        item(index, label, look)
            .on_mouse_move(highlighter(&self.sidebar, index))
            .relative()
            .child(caret(palette))
            .children(nested)
            .into_any_element()
    }

    fn nested(
        &self,
        entries: &[MenuEntry],
    ) -> Div {
        let list = MenuList {
            sidebar: self.sidebar.clone(),
            entries: entries.to_vec(),
            highlighted: None,
            submenu: None,
            focus: None,
        };
        div().absolute().left_full().top(SUBMENU_RISE).child(list)
    }
}

fn separator(palette: &Palette) -> Div {
    div()
        .mx(SPACE_MD)
        .my(SPACE_XS)
        .h(px(1.0))
        .bg(palette.border())
}

fn caret(palette: &Palette) -> Div {
    let icon = Icon::new(IconName::CaretRight)
        .size(ICON_LARGE)
        .color(palette.foreground().opacity(CARET_ALPHA));
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

fn item(
    index: usize,
    label: &str,
    look: ItemLook,
) -> Stateful<Div> {
    let palette = look.palette;
    let (fill, text) = match (look.tone, look.active) {
        (Tone::Destructive, true) => (
            palette.destructive().opacity(DESTRUCTIVE_TINT),
            palette.destructive(),
        ),
        (Tone::Destructive, false) => (gpui::transparent_black(), palette.destructive()),
        (Tone::Normal, true) => (palette.accent(), palette.accent_foreground()),
        (Tone::Normal, false) => (gpui::transparent_black(), palette.foreground()),
    };
    let icon = look.icon.map(|name| {
        let glyph = Icon::new(name).size(ICON_LARGE).color(text);
        div().mx(ICON_PULL).opacity(ICON_ALPHA).child(glyph)
    });
    div()
        .id(("sidebar-menu-item", index))
        .flex()
        .items_center()
        .gap(SPACE_MD)
        .min_h(px(ROW_MIN_HEIGHT))
        .px(SPACE_MD)
        .py(SPACE_XS)
        .rounded(radius::SM)
        .bg(fill)
        .text_color(text)
        .font_weight(FontWeight::NORMAL)
        .cursor_pointer()
        .children(icon)
        .child(label.to_owned())
        .children(check_slot(look.checked, palette))
}

fn check_slot(
    checked: Option<Check>,
    palette: &Palette,
) -> Option<Div> {
    let check = checked?;
    let slot = div().ml_auto().flex().justify_center().size(ICON_LARGE);
    if check == Check::Off {
        return Some(slot);
    }
    let mark = Icon::new(IconName::Check)
        .size(ICON_MEDIUM)
        .color(palette.primary());
    Some(slot.child(mark))
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point, px};
    use ui::IconName;

    use super::*;
    use crate::demo_data::DemoData;
    use crate::row_id::RowId;
    use crate::test_support::open;

    #[gpui::test]
    fn hovering_an_entry_highlights_it_and_clicking_applies_it(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let entries = vec![MenuEntry::action(
            "Keep",
            IconName::Check,
            Intent::Keep(RowId::from("chat")),
        )];
        sidebar.update_in(cx, |this, window, cx| {
            this.open_menu((entries, point(px(50.0), px(200.0))), window, cx);
        });
        cx.run_until_parked();
        sidebar.update(cx, |this, cx| this.highlight(Some(0), cx));
        let highlighted = sidebar.read_with(cx, |this, _| {
            this.menu
                .as_ref()
                .and_then(|menu| menu.cursor.highlighted())
        });
        assert_eq!(highlighted, Some(0));
    }
}
