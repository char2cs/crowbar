//! The desktop's own context menu, which the shell installs at start in place of the drawn one.

mod native_entry;
mod native_request;

use std::rc::Rc;

use gpui::{App, Global, Pixels, Point, Task, Window};

pub use native_entry::NativeEntry;
pub use native_request::NativeRequest;

use crate::floating::menu::menu_item::{MenuAction, MenuItem};
use crate::foundation::opaque_debug;

/// The chosen entry: its index in the menu, then its index in each submenu it lies in.
pub type MenuPath = Vec<usize>;

/// Pops a [`NativeRequest`] up over a window; its task ends with the chosen entry, none when
/// the menu is dismissed.
pub type PopupMenu = Rc<dyn Fn(NativeRequest, &Window, &mut App) -> Task<Option<MenuPath>>>;

type Run = Rc<dyn Fn(&mut Window, &mut App)>;

/// The installed native menu, a global; context menus are drawn in the window until one is
/// installed.
#[derive(Clone)]
pub struct NativeMenu(PopupMenu);

opaque_debug!(NativeMenu);

impl Global for NativeMenu {}

impl NativeMenu {
    /// A native menu that pops up with `popup`.
    #[must_use]
    pub fn new(popup: PopupMenu) -> Self {
        Self(popup)
    }

    pub(crate) fn installed(cx: &App) -> Option<Self> {
        cx.try_global::<Self>().cloned()
    }

    pub(crate) fn open(
        &self,
        items: &Rc<[MenuItem]>,
        at: Point<Pixels>,
        (window, cx): (&Window, &mut App),
    ) {
        let request = NativeRequest {
            entries: entries(items),
            at,
        };
        let chosen = (self.0)(request, window, cx);
        let items = Rc::clone(items);
        window
            .spawn(cx, async move |cx| {
                let path = chosen.await?;
                let run = action_at(&items, &path)?;
                cx.update(|window, cx| run(window, cx)).ok()
            })
            .detach();
    }
}

fn entries(items: &[MenuItem]) -> Vec<NativeEntry> {
    items.iter().map(entry).collect()
}

fn entry(item: &MenuItem) -> NativeEntry {
    match item {
        MenuItem::Action(action) => action_entry(item, action),
        MenuItem::Separator => NativeEntry::Separator,
        MenuItem::Label(text) => NativeEntry::Item {
            label: text.clone(),
            enabled: false,
            checked: false,
        },
    }
}

fn action_entry(
    item: &MenuItem,
    action: &MenuAction,
) -> NativeEntry {
    let (label, enabled) = (action.label.clone(), !action.disabled);
    match item.submenu_items() {
        Some(nested) => NativeEntry::Submenu {
            label,
            enabled,
            entries: entries(&nested),
        },
        None => NativeEntry::Item {
            label,
            enabled,
            checked: action.checked,
        },
    }
}

fn action_at(
    items: &[MenuItem],
    path: &[usize],
) -> Option<Run> {
    let (first, rest) = path.split_first()?;
    let item = items.get(*first)?;
    if !rest.is_empty() {
        return action_at(&item.submenu_items()?, rest);
    }
    let MenuItem::Action(action) = item else {
        return None;
    };
    item.is_selectable().then(|| Rc::clone(&action.on_select))
}

#[cfg(test)]
mod tests {
    use std::cell::{Cell, RefCell};

    use gpui::{Modifiers, MouseButton, Styled as _, TestAppContext, div, point, px};

    use super::*;
    use crate::floating::menu::ContextMenu;
    use crate::test_support::probe::open;

    type Asked = Rc<RefCell<Vec<NativeRequest>>>;

    fn answering(
        cx: &mut TestAppContext,
        path: Option<MenuPath>,
    ) -> Asked {
        let asked = Asked::default();
        let heard = Rc::clone(&asked);
        let popup: PopupMenu = Rc::new(move |request, _, _| {
            heard.borrow_mut().push(request);
            Task::ready(path.clone())
        });
        cx.update(|cx| cx.set_global(NativeMenu::new(popup)));
        asked
    }

    fn rows(chosen: &Rc<Cell<u32>>) -> Vec<MenuItem> {
        let (outer, inner) = (Rc::clone(chosen), Rc::clone(chosen));
        let rename = MenuAction::new("Rename", move |_, _| outer.set(outer.get() + 1));
        let deep = MenuAction::new("main", move |_, _| inner.set(inner.get() + 10));
        let nested = vec![MenuItem::Action(deep)];
        vec![
            MenuItem::Action(rename.shortcut("F2").destructive()),
            MenuItem::Separator,
            MenuItem::Action(MenuAction::submenu("Move to…", nested)),
            MenuItem::Action(MenuAction::new("Off", |_, _| {}).disabled()),
        ]
    }

    fn item(label: &'static str) -> NativeEntry {
        NativeEntry::Item {
            label: label.into(),
            enabled: true,
            checked: false,
        }
    }

    fn right_click(
        cx: &mut TestAppContext,
        chosen: &Rc<Cell<u32>>,
    ) -> Option<gpui::Bounds<Pixels>> {
        let chosen = Rc::clone(chosen);
        let cx = open(cx, move |_, _| {
            let area = div().w(px(200.0)).h(px(100.0));
            ContextMenu::new("ctx", area, rows(&chosen))
        });
        let at = point(px(30.0), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(at, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        cx.debug_bounds("ui-menu")
    }

    #[gpui::test]
    fn a_right_click_asks_for_a_native_menu_of_plain_rows_at_the_pointer(cx: &mut TestAppContext) {
        let asked = answering(cx, None);
        let drawn = right_click(cx, &Rc::new(Cell::new(0)));
        let submenu = NativeEntry::Submenu {
            label: "Move to…".into(),
            enabled: true,
            entries: vec![item("main")],
        };
        let off = NativeEntry::Item {
            label: "Off".into(),
            enabled: false,
            checked: false,
        };
        let entries = vec![item("Rename"), NativeEntry::Separator, submenu, off];
        let want = NativeRequest {
            entries,
            at: point(px(30.0), px(20.0)),
        };
        assert_eq!(*asked.borrow(), [want]);
        assert_eq!(drawn, None, "nothing is drawn in the window");
    }

    #[gpui::test]
    fn the_chosen_entry_runs_its_action_even_inside_a_submenu(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        answering(cx, Some(vec![2, 0]));
        right_click(cx, &chosen);
        assert_eq!(chosen.get(), 10);
    }

    #[test]
    fn a_dismissed_menu_or_a_disabled_or_missing_entry_runs_nothing() {
        let items = rows(&Rc::new(Cell::new(0)));
        for path in [vec![], vec![1], vec![3], vec![9], vec![2, 4]] {
            assert!(action_at(&items, &path).is_none(), "{path:?}");
        }
        assert!(action_at(&items, &[0]).is_some());
    }
}
