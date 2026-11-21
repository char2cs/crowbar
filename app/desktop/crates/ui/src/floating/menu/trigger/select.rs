//! A dropdown that picks one value from a list.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement, ParentElement, Pixels,
    Point, RenderOnce, SharedString, Styled, Window, div, point, px,
};

use crate::floating::menu::menu_item::MenuAction;
use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::trigger::dropdown_menu::DropdownMenu;
use crate::floating::menu::{OPTION_CHECK, OPTION_GAP, OPTION_HEIGHT, OPTION_START};
use crate::foundation::Appearance;
use crate::foundation::ControlScale;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IndexHandler;
use crate::foundation::Palette;
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
const MENU_EDGE: Pixels = px(5.0);
const OPTION_REACH: Pixels = px(30.0);
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
    ) -> gpui::Stateful<gpui::Div> {
        let palette = theme.palette(self.surface);
        let fill = match theme.appearance() {
            Appearance::Light => palette.background(),
            Appearance::Dark => palette.input().opacity(opacity::DARK_INPUT),
        };
        let (label, color) = self.shown(palette);
        let caret = Icon::new(IconName::CaretUpDown)
            .size(breakpoint.icon())
            .color(palette.foreground().opacity(CARET_OPACITY));
        div()
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

fn over_chosen(
    chosen: usize,
    (trigger, row): (Pixels, Pixels),
) -> Point<Pixels> {
    let rows = row * index_f32(chosen);
    let x = spacing::STEP_2_5 - (MENU_EDGE + OPTION_START + OPTION_CHECK + OPTION_GAP);
    point(x, (trigger - row) / 2.0 - (MENU_EDGE + rows))
}

fn index_f32(index: usize) -> f32 {
    u16::try_from(index).map_or(f32::from(u16::MAX), f32::from)
}

impl RenderOnce for Select {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let breakpoint = ControlScale::of(window);
        let trigger = self.trigger(*Theme::of(cx), breakpoint);
        let growth = breakpoint.growth();
        let heights = (self.height + growth, OPTION_HEIGHT + growth);
        let items: Vec<MenuItem> = self
            .options
            .iter()
            .enumerate()
            .map(|(index, option)| self.item(index, option))
            .collect();
        let chosen = self.selected.unwrap_or(0);
        DropdownMenu::new(self.id, trigger, items)
            .options_over(self.selected, (over_chosen(chosen, heights), OPTION_REACH))
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

    #[gpui::test]
    fn the_list_opens_over_the_trigger_with_the_chosen_row_on_it(cx: &mut TestAppContext) {
        let cx = crate::test_support::probe::open(cx, |_, _| {
            div()
                .pt(px(200.0))
                .pl(px(100.0))
                .child(Select::new("s", options(), Some(2)).compact())
        });
        cx.simulate_click(point(px(120.0), px(210.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        let chosen = cx.debug_bounds("menu-row-2").unwrap_or_default();
        assert_eq!(chosen.top(), px(200.0), "{chosen:?}");
        assert_eq!(chosen.size.height, px(28.0), "{chosen:?}");
        assert_eq!(menu.left(), px(100.0 - 27.0), "{menu:?}");
        assert_eq!(menu.size.width, px(144.0 + 30.0), "{menu:?}");
    }

    #[gpui::test]
    fn a_filling_trigger_opens_a_list_wider_than_itself(cx: &mut TestAppContext) {
        let cx = crate::test_support::probe::open(cx, |_, _| {
            div()
                .w(px(300.0))
                .child(Select::new("s", options(), Some(0)).fill())
        });
        cx.simulate_click(point(px(150.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        assert_eq!(menu.size.width, px(300.0 + 30.0), "{menu:?}");
    }
}
