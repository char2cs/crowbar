//! How a menu draws its rows: plain (icon, label, trailing hint or check) or a select's options.

use gpui::{AnyElement, IntoElement};

use crate::floating::menu::Menu;
use crate::floating::menu::menu_item::MenuAction;
use crate::floating::menu::option_row::OptionRow;
use crate::floating::menu::row::Row;
use crate::floating::menu::row_handlers::RowHandlers;
use crate::foundation::Pose;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum RowStyle {
    #[default]
    Plain,
    Option,
}

impl Menu {
    #[must_use]
    pub(crate) fn row_style(
        mut self,
        style: RowStyle,
    ) -> Self {
        self.row_style = style;
        self
    }

    #[must_use]
    pub(crate) fn highlighting(
        mut self,
        index: Option<usize>,
    ) -> Self {
        self.highlighted = index;
        self
    }

    pub(crate) fn action_row(
        &self,
        (index, action): (usize, &MenuAction),
        handlers: &RowHandlers,
    ) -> AnyElement {
        let highlight = Pose::from(self.highlighted == Some(index));
        let (action, handlers) = (action.clone(), handlers.clone());
        match self.row_style {
            RowStyle::Plain => Row::new(index, action, highlight, handlers).into_any_element(),
            RowStyle::Option => {
                OptionRow::new(index, action, highlight, handlers).into_any_element()
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{AppContext as _, TestAppContext, VisualTestContext};

    use super::*;
    use crate::floating::menu::menu_item::MenuItem;
    use crate::test_support::probe::open;

    fn items() -> Rc<[MenuItem]> {
        let row = |label: &'static str| MenuItem::Action(MenuAction::new(label, |_, _| {}));
        Rc::from(vec![row("Light"), row("Dark")])
    }

    fn menu(
        style: RowStyle,
        window: &mut gpui::Window,
        cx: &mut gpui::App,
    ) -> gpui::Entity<Menu> {
        cx.new(|cx| Menu::new(items(), window, cx).row_style(style))
    }

    fn opened(
        cx: &mut TestAppContext,
        style: RowStyle,
    ) -> &mut VisualTestContext {
        open(cx, move |window, cx| menu(style, window, cx))
    }

    #[gpui::test]
    fn option_style_draws_option_rows(cx: &mut TestAppContext) {
        let cx = opened(cx, RowStyle::Option);
        assert!(cx.debug_bounds("option-label-1").is_some());
    }

    #[gpui::test]
    fn plain_style_keeps_plain_rows(cx: &mut TestAppContext) {
        let cx = opened(cx, RowStyle::Plain);
        assert!(cx.debug_bounds("option-label-1").is_none());
    }
}
