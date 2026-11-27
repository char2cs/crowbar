//! The desktop's own context menu, which the shell installs at start.

mod entries;
mod menu_edge;
mod native_entry;
mod native_request;

use std::rc::Rc;

use std::cell::Cell;

use gpui::{
    App, Bounds, Global, InteractiveElement, IntoElement, MouseButton, Pixels, Point, Styled as _,
    Task, Window, canvas,
};

pub use menu_edge::MenuEdge;
pub use native_entry::NativeEntry;
pub use native_request::NativeRequest;

use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::native_menu::entries::{action_at, entries};
use crate::foundation::opaque_debug;

/// The chosen entry: its index in the menu, then its index in each submenu it lies in.
pub type MenuPath = Vec<usize>;

/// Pops a [`NativeRequest`] up over a window; its task ends with the chosen entry, none when
/// the menu is dismissed.
pub type PopupMenu = Rc<dyn Fn(NativeRequest, &Window, &mut App) -> Task<Option<MenuPath>>>;

/// The installed native menu, a global; menus open nothing until one is installed.
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

    /// The installed native menu, if the shell installed one.
    #[must_use]
    pub fn installed(cx: &App) -> Option<Self> {
        cx.try_global::<Self>().cloned()
    }

    /// Pops `request` up over `window`; the task ends with the chosen entry.
    pub fn popup(
        &self,
        request: NativeRequest,
        window: &Window,
        cx: &mut App,
    ) -> Task<Option<MenuPath>> {
        (self.0)(request, window, cx)
    }

    /// Pops `items` up with `edge` at `at` in the installed menu and runs the chosen row's action.
    pub fn open(
        items: &Rc<[MenuItem]>,
        anchor: (Point<Pixels>, MenuEdge),
        (window, cx): (&Window, &mut App),
    ) {
        let (at, edge) = anchor;
        let request = NativeRequest {
            entries: entries(items),
            at,
            edge,
        };
        let Some(native) = Self::installed(cx) else {
            return;
        };
        let chosen = native.popup(request, window, cx);
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

pub(crate) fn trigger_probe() -> (Rc<Cell<Bounds<Pixels>>>, impl IntoElement) {
    let laid_out: Rc<Cell<Bounds<Pixels>>> = Rc::default();
    let measured = Rc::clone(&laid_out);
    let probe = canvas(move |bounds, _, _| measured.set(bounds), |_, (), _, _| {});
    (laid_out, probe.absolute().top_0().left_0().size_full())
}

/// Calls `open` with the pointer when `element` is right-clicked: on release for a GTK menu, on
/// press otherwise.
pub fn on_context_click<E: InteractiveElement>(
    element: E,
    open: impl Fn(Point<Pixels>, &mut Window, &mut App) + 'static,
) -> E {
    let on_press: Rc<Open> = Rc::new(open);
    let on_release = Rc::clone(&on_press);
    // PLATFORM: linux the press holds an implicit X grab on the shell window, so GTK can only take the pointer for its menu once the button is up; macos opens menus on the press.
    element
        .on_mouse_down(MouseButton::Right, move |event, window, cx| {
            pressed(&on_press, event.position, (window, cx));
        })
        .on_mouse_up(MouseButton::Right, move |event, window, cx| {
            released(&on_release, event.position, (window, cx));
        })
}

type Open = dyn Fn(Point<Pixels>, &mut Window, &mut App);

fn pressed(
    open: &Rc<Open>,
    at: Point<Pixels>,
    (window, cx): (&mut Window, &mut App),
) {
    if !opens_on_release(cx) {
        open(at, window, cx);
    }
}

fn released(
    open: &Rc<Open>,
    at: Point<Pixels>,
    (window, cx): (&mut Window, &mut App),
) {
    if opens_on_release(cx) {
        open(at, window, cx);
    }
}

fn opens_on_release(cx: &App) -> bool {
    cfg!(target_os = "linux") && NativeMenu::installed(cx).is_some()
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, div, point, px};

    use super::*;
    use crate::floating::menu::ContextMenu;
    use crate::floating::menu::menu_item::MenuAction;
    use crate::test_support::menus::answering;
    use crate::test_support::probe::open;

    fn rows(chosen: &Rc<Cell<u32>>) -> Vec<MenuItem> {
        let (outer, inner) = (Rc::clone(chosen), Rc::clone(chosen));
        let rename = MenuAction::new("Rename", move |_, _| outer.set(outer.get() + 1));
        let deep = MenuAction::new("main", move |_, _| inner.set(inner.get() + 10));
        let nested = vec![MenuItem::Action(deep)];
        vec![
            MenuItem::Action(rename),
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
    ) {
        let chosen = Rc::clone(chosen);
        let cx = open(cx, move |_, _| {
            let area = div().w(px(200.0)).h(px(100.0));
            ContextMenu::new("ctx", area, rows(&chosen))
        });
        let at = point(px(30.0), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(at, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
    }

    #[gpui::test]
    fn a_right_click_asks_for_a_native_menu_of_plain_rows_at_the_pointer(cx: &mut TestAppContext) {
        let asked = answering(cx, None);
        right_click(cx, &Rc::new(Cell::new(0)));
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
            edge: MenuEdge::Start,
        };
        assert_eq!(*asked.borrow(), [want]);
    }

    #[gpui::test]
    fn a_placed_menu_starts_under_the_centre_of_its_trigger_as_develops_tab_menu(
        cx: &mut TestAppContext
    ) {
        let asked = answering(cx, None);
        let cx = open(cx, |_, _| {
            let area = div().w(px(200.0)).h(px(100.0));
            let rows = vec![MenuItem::Action(MenuAction::new("Close", |_, _| {}))];
            ContextMenu::new("tab", area, rows).placement(crate::Placement::BelowCentre)
        });
        let at = point(px(30.0), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(at, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        let area = cx.debug_bounds("probe").unwrap_or_default();
        let opened_at = asked.borrow().first().map(|request| request.at);
        let under = point(area.origin.x + px(100.0), area.bottom() + px(4.0));
        assert_eq!(opened_at, Some(under));
    }

    #[gpui::test]
    fn the_chosen_entry_runs_its_action_even_inside_a_submenu(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        answering(cx, Some(vec![2, 0]));
        right_click(cx, &chosen);
        assert_eq!(chosen.get(), 10);
    }

    fn press_then_release(cx: &mut TestAppContext) -> (u32, u32) {
        let heard = Rc::new(Cell::new(0));
        let count = Rc::clone(&heard);
        let cx = open(cx, move |_, _| {
            let count = Rc::clone(&count);
            let area = div().w(px(200.0)).h(px(100.0));
            on_context_click(area, move |_, _, _| count.set(count.get() + 1))
        });
        let at = point(px(30.0), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        let pressed = heard.get();
        cx.simulate_mouse_up(at, MouseButton::Right, Modifiers::none());
        (pressed, heard.get())
    }

    #[gpui::test]
    fn without_a_native_menu_a_context_click_is_heard_on_the_press(cx: &mut TestAppContext) {
        assert_eq!(press_then_release(cx), (1, 1));
    }

    #[gpui::test]
    fn with_a_native_menu_a_context_click_opens_when_the_desktop_s_menus_do(
        cx: &mut TestAppContext
    ) {
        answering(cx, None);
        assert_eq!(press_then_release(cx), heard_here());
    }

    // PLATFORM: linux GTK opens on the release, macos on the press.
    fn heard_here() -> (u32, u32) {
        if cfg!(target_os = "linux") {
            return (0, 1);
        }
        (1, 1)
    }
}
