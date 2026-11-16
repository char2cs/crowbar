//! A dropdown that picks one value from a list.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement, ParentElement, Pixels,
    RenderOnce, SharedString, Styled, Window, div, px,
};

use crate::appearance::Appearance;
use crate::dropdown_menu::DropdownMenu;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::index_handler::IndexHandler;
use crate::menu_action::MenuAction;
use crate::menu_item::MenuItem;
use crate::opacity;
use crate::opaque_debug::opaque_debug;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::spacing;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

const DEFAULT_WIDTH: f32 = 144.0;
const STANDARD_HEIGHT: f32 = 32.0;
const COMPACT_HEIGHT: f32 = 28.0;
const CARET_OPACITY: f32 = 0.8;
const PLACEHOLDER_OPACITY: f32 = 0.72;

/// A select box: the trigger shows the chosen option, the popup lists all of them with a check on the current one.
#[derive(IntoElement)]
pub struct Select {
    id: ElementId,
    options: Vec<SharedString>,
    selected: Option<usize>,
    placeholder: SharedString,
    width: Pixels,
    fills: bool,
    height: Pixels,
    surface: Surface,
    on_change: Option<IndexHandler>,
}

impl Select {
    /// A select over `options` with `selected` chosen.
    pub fn new(
        id: impl Into<ElementId>,
        options: Vec<SharedString>,
        selected: Option<usize>,
    ) -> Self {
        Self {
            id: id.into(),
            options,
            selected,
            placeholder: SharedString::default(),
            width: px(DEFAULT_WIDTH),
            fills: false,
            height: px(STANDARD_HEIGHT),
            surface: Surface::default(),
            on_change: None,
        }
    }

    /// Text shown while nothing is selected.
    #[must_use]
    pub fn placeholder(
        mut self,
        placeholder: impl Into<SharedString>,
    ) -> Self {
        self.placeholder = placeholder.into();
        self
    }

    /// Sets the width of the trigger and the minimum width of the popup.
    #[must_use]
    pub fn width(
        mut self,
        width: Pixels,
    ) -> Self {
        self.width = width;
        self
    }

    /// Stretches the trigger to the width of its parent; the popup keeps the `width` as its minimum.
    #[must_use]
    pub fn fill(mut self) -> Self {
        self.fills = true;
        self
    }

    /// Makes the trigger 28px tall, as in a row of settings.
    #[must_use]
    pub fn compact(mut self) -> Self {
        self.height = px(COMPACT_HEIGHT);
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the index of the chosen option.
    #[must_use]
    pub fn on_change(
        mut self,
        handler: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_change = Some(Rc::new(handler));
        self
    }
}

impl Select {
    fn trigger(
        &self,
        theme: Theme,
    ) -> gpui::Stateful<gpui::Div> {
        let palette = theme.palette(self.surface);
        let fill = match theme.appearance() {
            Appearance::Light => palette.background(),
            Appearance::Dark => palette.input().opacity(opacity::DARK_INPUT),
        };
        let (label, color) = self.shown(palette);
        let caret =
            Icon::new(IconName::CaretUpDown).color(palette.foreground().opacity(CARET_OPACITY));
        div()
            .id((self.id.clone(), "trigger"))
            .flex()
            .items_center()
            .justify_between()
            .gap(spacing::STEP_2)
            .when(self.fills, Styled::w_full)
            .when(!self.fills, |trigger| trigger.w(self.width))
            .min_h(self.height)
            .px(spacing::STEP_2_5)
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.input())
            .bg(fill)
            .text_size(text_size::SM)
            .font_family(theme.ui_family())
            .font_weight(FontWeight::NORMAL)
            .text_color(color)
            .cursor_pointer()
            .shadow(shadow::xs(opacity::PERCENT_5))
            .child(
                div()
                    .flex_1()
                    .overflow_hidden()
                    .whitespace_nowrap()
                    .child(label),
            )
            .child(caret)
    }

    fn shown(
        &self,
        palette: &Palette,
    ) -> (SharedString, Hsla) {
        match self.selected.and_then(|index| self.options.get(index)) {
            Some(label) => (label.clone(), palette.foreground()),
            None => (
                self.placeholder.clone(),
                palette.muted_foreground().opacity(PLACEHOLDER_OPACITY),
            ),
        }
    }

    fn marked(
        &self,
        index: usize,
        action: MenuAction,
    ) -> MenuAction {
        if self.selected == Some(index) {
            return action.checked();
        }
        action
    }

    fn item(
        &self,
        index: usize,
        option: &SharedString,
    ) -> MenuItem {
        let handler = self.on_change.clone();
        let action = MenuAction::new(option.clone(), move |window, cx| {
            forward(handler.as_ref(), index, window, cx);
        });
        MenuItem::Action(self.marked(index, action))
    }
}

fn forward(
    handler: Option<&IndexHandler>,
    index: usize,
    window: &mut Window,
    cx: &mut App,
) {
    if let Some(handler) = handler {
        handler(index, window, cx);
    }
}

impl RenderOnce for Select {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let trigger = self.trigger(*Theme::of(cx));
        let items: Vec<MenuItem> = self
            .options
            .iter()
            .enumerate()
            .map(|(index, option)| self.item(index, option))
            .collect();
        DropdownMenu::new(self.id, trigger, items).min_width(self.width)
    }
}

opaque_debug!(Select);

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::measure;

    fn options() -> Vec<SharedString> {
        vec!["System".into(), "Light".into(), "Dark".into()]
    }

    #[gpui::test]
    fn trigger_is_thirty_two_pixels_high_and_as_wide_as_requested(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            Select::new("s", options(), Some(1)).width(px(160.0))
        });
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (160.0, 32.0)
        );
    }

    #[gpui::test]
    fn default_width_is_one_hundred_forty_four(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            Select::new("s", options(), None).placeholder("Choose")
        });
        assert_eq!(f32::from(bounds.size.width), 144.0);
    }

    #[gpui::test]
    fn compact_triggers_are_28px_tall(cx: &mut TestAppContext) {
        let compact = measure(cx, |_, _| {
            Select::new("s", vec!["A".into()], Some(0)).compact()
        });
        let standard = measure(cx, |_, _| Select::new("s", vec!["A".into()], Some(0)));
        assert_eq!(
            (
                f32::from(compact.size.height),
                f32::from(standard.size.height)
            ),
            (28.0, 32.0)
        );
    }
}
