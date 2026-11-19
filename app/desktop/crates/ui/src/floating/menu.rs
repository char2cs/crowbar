//! A keyboard-navigable list of actions in a popup frame.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Context, DismissEvent, Entity, EventEmitter, FocusHandle, Focusable,
    FontWeight, InteractiveElement, IntoElement, ParentElement, Pixels, Render, Styled, Window,
    div, px,
};

use crate::floating::menu::navigation::actions::{CONTEXT, SelectNext, SelectPrevious};
use crate::floating::menu::nested::level::Level;
use crate::floating::menu::nested::navigation::Navigation;
use crate::floating::menu::nested::open_submenu::OpenSubmenu;
use crate::floating::menu::row::Row;
use crate::floating::menu::row_handlers::{RowHandler, RowHandlers};
use crate::foundation::Direction;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;

/// The popup frame with its rows; emits [`DismissEvent`] when it should close.
pub struct Menu {
    pub(crate) items: Rc<[MenuItem]>,
    pub(crate) highlighted: Option<usize>,
    min_width: Pixels,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) level: Level,
    pub(crate) submenu: Option<OpenSubmenu<Menu>>,
}

mod menu_item;
mod navigation;
mod nested;
mod row;
mod row_handlers;
mod trigger;

pub use menu_item::{MenuAction, MenuItem};
pub(crate) use navigation::bindings;
pub use trigger::{ContextMenu, DropdownMenu, Select};

impl EventEmitter<DismissEvent> for Menu {}

impl EventEmitter<Navigation> for Menu {}

impl Focusable for Menu {
    fn focus_handle(
        &self,
        _cx: &App,
    ) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Menu {
    pub(crate) const DEFAULT_MIN_WIDTH: Pixels = px(128.0);

    /// Builds a menu and takes focus so the arrow keys work at once.
    pub fn new(
        items: impl Into<Rc<[MenuItem]>>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self::at_level(items.into(), Level::Root, window, cx)
    }

    pub(crate) fn nested(
        items: Rc<[MenuItem]>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self::at_level(items, Level::Nested, window, cx)
    }

    fn at_level(
        items: Rc<[MenuItem]>,
        level: Level,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle);
        // PLATFORM: gpui gives focus to the pressed ancestor after the click handler ran; claim it back.
        let later = focus_handle.clone();
        window.defer(cx, move |window, _| window.focus(&later));
        Self {
            items,
            highlighted: None,
            min_width: Self::DEFAULT_MIN_WIDTH,
            focus_handle,
            level,
            submenu: None,
        }
    }

    /// Widens the menu to at least `width`, as selects do to match their trigger.
    #[must_use]
    pub fn min_width(
        mut self,
        width: Pixels,
    ) -> Self {
        self.min_width = width;
        self
    }

    fn hover_handler(entity: Entity<Self>) -> RowHandler {
        Rc::new(move |index, window, cx| {
            entity.update(cx, |this, cx| this.highlight(index, window, cx));
        })
    }

    fn choose_handler(entity: Entity<Self>) -> RowHandler {
        Rc::new(move |index, window, cx| {
            entity.update(cx, |this, cx| this.choose(index, window, cx));
        })
    }

    fn handlers(cx: &Context<'_, Self>) -> RowHandlers {
        RowHandlers {
            hover: Self::hover_handler(cx.entity()),
            choose: Self::choose_handler(cx.entity()),
        }
    }

    fn entry(
        &self,
        (index, item): (usize, &MenuItem),
        palette: &Palette,
        handlers: &RowHandlers,
    ) -> AnyElement {
        match item {
            MenuItem::Action(action) => {
                let highlight = Pose::from(self.highlighted == Some(index));
                let row = Row::new(index, action.clone(), highlight, handlers.clone());
                self.with_submenu(index, row.into_any_element())
            },
            MenuItem::Separator => div()
                .my(spacing::STEP_1)
                .mx(-spacing::STEP_1)
                .h(px(1.0))
                .bg(palette.border())
                .into_any_element(),
            MenuItem::Label(text) => div()
                .px(spacing::STEP_1_5)
                .py(spacing::STEP_1)
                .text_size(text_size::XS)
                .font_weight(FontWeight::MEDIUM)
                .text_color(palette.muted_foreground())
                .child(text.clone())
                .into_any_element(),
        }
    }
}

impl Render for Menu {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let handlers = Self::handlers(cx);
        let rows = self
            .items
            .iter()
            .enumerate()
            .map(|entry| self.entry(entry, palette, &handlers));
        div()
            .id("ui-menu")
            .debug_selector(|| "ui-menu".into())
            .key_context(CONTEXT)
            .track_focus(&self.focus_handle)
            .on_action(cx.listener(|this, _: &SelectNext, _, cx| {
                this.move_highlight(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &SelectPrevious, _, cx| {
                this.move_highlight(Direction::Backward, cx);
            }))
            .on_action(cx.listener(Self::confirm))
            .on_action(cx.listener(Self::dismiss))
            .on_action(cx.listener(Self::close_submenu))
            .on_action(cx.listener(Self::open_highlighted))
            .relative()
            .occlude()
            .flex()
            .flex_col()
            .min_w(self.min_width)
            .p(spacing::STEP_1)
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.foreground().opacity(opacity::PERCENT_10))
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(Theme::of(cx).ui_family())
            .shadow(shadow::md(opacity::PERCENT_10))
            .children(rows)
    }
}

opaque_debug!(Menu);

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{DismissEvent, Entity, TestAppContext, VisualTestContext};

    use super::*;
    use crate::floating::menu::menu_item::MenuAction;
    use crate::foundation::Motion;

    pub(super) type Log = Rc<RefCell<Vec<&'static str>>>;

    fn items(log: &Log) -> Vec<MenuItem> {
        let record = |name: &'static str| {
            let log = Rc::clone(log);
            move |_: &mut Window, _: &mut App| log.borrow_mut().push(name)
        };
        vec![
            MenuItem::Action(MenuAction::new("First", record("first"))),
            MenuItem::Action(MenuAction::new("Off", record("off")).disabled()),
            MenuItem::Separator,
            MenuItem::Action(MenuAction::new("Last", record("last"))),
        ]
    }

    fn bump(counter: &RefCell<u32>) {
        *counter.borrow_mut() += 1;
    }

    pub(super) type Opened<'a> = (Entity<Menu>, &'a mut VisualTestContext, Rc<RefCell<u32>>);

    pub(super) fn open_menu<'a>(
        cx: &'a mut TestAppContext,
        log: &Log,
    ) -> Opened<'a> {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
            cx.bind_keys(bindings());
        });
        let entries = items(log);
        let (menu, cx) = cx.add_window_view(move |window, cx| Menu::new(entries, window, cx));
        let dismissed = Rc::new(RefCell::new(0));
        let counter = Rc::clone(&dismissed);
        cx.update(|_, cx| {
            cx.subscribe(&menu, move |_, _: &DismissEvent, _| bump(&counter))
                .detach();
        });
        (menu, cx, dismissed)
    }

    #[gpui::test]
    fn escape_dismisses_without_running_anything(cx: &mut TestAppContext) {
        let log = Log::default();
        let (_, cx, dismissed) = open_menu(cx, &log);
        cx.simulate_keystrokes("down escape");
        assert!(log.borrow().is_empty());
        assert_eq!(*dismissed.borrow(), 1);
    }
}
