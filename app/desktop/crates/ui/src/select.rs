//! A dropdown that picks one value from a list.

use std::rc::Rc;

use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement, Pixels, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::appearance::Appearance;
use crate::dropdown_menu::DropdownMenu;
use crate::font::UI_FAMILY;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::index_handler::IndexHandler;
use crate::menu_action::MenuAction;
use crate::menu_item::MenuItem;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

const DEFAULT_WIDTH: f32 = 144.0;
const CARET_OPACITY: f32 = 0.8;

/// A select box: the trigger shows the chosen option, the popup lists all of them with a check on the current one.
#[derive(IntoElement)]
pub struct Select {
    id: ElementId,
    options: Vec<SharedString>,
    selected: Option<usize>,
    placeholder: SharedString,
    width: Pixels,
    surface: Surface,
    on_change: Option<IndexHandler>,
}

impl Select {
    /// A select over `options` with `selected` chosen.
    pub fn new(id: impl Into<ElementId>, options: Vec<SharedString>, selected: Option<usize>) -> Self {
        Self {
            id: id.into(),
            options,
            selected,
            placeholder: SharedString::default(),
            width: px(DEFAULT_WIDTH),
            surface: Surface::default(),
            on_change: None,
        }
    }

    /// Text shown while nothing is selected.
    #[must_use]
    pub fn placeholder(mut self, placeholder: impl Into<SharedString>) -> Self {
        self.placeholder = placeholder.into();
        self
    }

    /// Sets the width of the trigger and the minimum width of the popup.
    #[must_use]
    pub fn width(mut self, width: Pixels) -> Self {
        self.width = width;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the index of the chosen option.
    #[must_use]
    pub fn on_change(mut self, handler: impl Fn(usize, &mut Window, &mut App) + 'static) -> Self {
        self.on_change = Some(Rc::new(handler));
        self
    }
}

impl RenderOnce for Select {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let fill = match theme.appearance() {
            Appearance::Light => palette.background(),
            Appearance::Dark => palette.input().opacity(0.32),
        };
        let current = self.selected.and_then(|index| self.options.get(index));
        let (label, color) = match current {
            Some(label) => (label.clone(), palette.foreground()),
            None => (self.placeholder.clone(), palette.muted_foreground().opacity(0.72)),
        };
        let trigger = div()
            .id((self.id.clone(), "trigger"))
            .flex()
            .items_center()
            .justify_between()
            .gap(px(8.0))
            .w(self.width)
            .min_h(px(32.0))
            .px(px(10.0))
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.input())
            .bg(fill)
            .text_size(text_size::SM)
            .font_family(UI_FAMILY)
            .font_weight(FontWeight::NORMAL)
            .text_color(color)
            .cursor_pointer()
            .shadow(shadow::xs(0.05))
            .child(div().flex_1().overflow_hidden().whitespace_nowrap().child(label))
            .child(Icon::new(IconName::CaretUpDown).color(palette.foreground().opacity(CARET_OPACITY)));
        let items: Vec<MenuItem> = self
            .options
            .iter()
            .enumerate()
            .map(|(index, option)| {
                let handler = self.on_change.clone();
                MenuItem::Action(
                    MenuAction::new(option.clone(), move |window, cx| {
                        if let Some(handler) = &handler {
                            handler(index, window, cx);
                        }
                    })
                    .checked(self.selected == Some(index)),
                )
            })
            .collect();
        DropdownMenu::new(self.id, trigger, items).min_width(self.width)
    }
}

#[cfg(test)]
#[path = "select_tests.rs"]
mod tests;
