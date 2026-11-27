//! A dropdown that picks one value from a list.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, ElementId, FocusHandle, FontWeight, Hsla, InteractiveElement, IntoElement, ParentElement,
    Pixels, RenderOnce, SharedString, Styled, Window, div, px,
};

use crate::floating::menu::menu_item::MenuAction;
use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::trigger::dropdown_menu::DropdownMenu;
use crate::foundation::Appearance;
use crate::foundation::ControlScale;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IndexHandler;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::spacing;

const DEFAULT_WIDTH: f32 = 144.0;
const STANDARD_HEIGHT: f32 = 32.0;
const COMPACT_HEIGHT: f32 = 28.0;
const CARET_OPACITY: f32 = 0.8;
const PLACEHOLDER_OPACITY: f32 = 0.72;

/// A select box: the trigger shows the chosen option, its menu lists all of them with a check on the current one.
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

mod ring;
mod style;

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
        breakpoint: ControlScale,
        focus: (&FocusHandle, Pose),
    ) -> gpui::Stateful<gpui::Div> {
        let palette = theme.palette(self.surface);
        let fill = match theme.appearance() {
            Appearance::Light => palette.background(),
            Appearance::Dark => palette.input().opacity(opacity::DARK_INPUT),
        };
        let (label, color) = self.shown(palette);
        let trigger = div()
            .id((self.id.clone(), "trigger"))
            .flex()
            .items_center()
            .justify_between()
            .gap(spacing::STEP_2)
            .when(self.fills, Styled::w_full)
            .when(!self.fills, |trigger| trigger.w(self.width))
            .min_w(px(DEFAULT_WIDTH))
            .min_h(self.height + breakpoint.growth())
            .px(spacing::STEP_2_5)
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.input())
            .bg(fill)
            .text_size(breakpoint.text())
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
            .child(caret(breakpoint, palette));
        ring::focusable(trigger, focus, palette.ring())
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

fn caret(
    breakpoint: ControlScale,
    palette: &Palette,
) -> Icon {
    Icon::new(IconName::CaretUpDown)
        .size(breakpoint.icon())
        .color(palette.foreground().opacity(CARET_OPACITY))
}

impl RenderOnce for Select {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let breakpoint = ControlScale::of(window);
        let focus = window.use_keyed_state((self.id.clone(), "trigger-focus"), cx, |_, cx| {
            cx.focus_handle()
        });
        let focus = focus.read(cx).clone();
        let rung = Pose::from(crate::Modality::rings(&focus, window, cx));
        let trigger = self.trigger(*Theme::of(cx), breakpoint, (&focus, rung));
        let items: Vec<MenuItem> = self
            .options
            .iter()
            .enumerate()
            .map(|(index, option)| self.item(index, option))
            .collect();
        DropdownMenu::new(self.id, trigger, items).trigger_focus(focus)
    }
}

opaque_debug!(Select);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::probe::measure;

    fn options() -> Vec<SharedString> {
        vec!["System".into(), "Light".into(), "Dark".into()]
    }

    #[gpui::test]
    fn default_width_is_one_hundred_forty_four(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            Select::new("s", options(), None).placeholder("Choose")
        });
        assert_eq!(f32::from(bounds.size.width), 144.0);
    }

    #[gpui::test]
    fn a_narrower_width_still_keeps_the_trigger_one_hundred_forty_four_wide(
        cx: &mut TestAppContext
    ) {
        let bounds = measure(cx, |_, _| {
            Select::new("s", options(), Some(0)).width(px(112.0))
        });
        assert_eq!(f32::from(bounds.size.width), 144.0);
    }
}
