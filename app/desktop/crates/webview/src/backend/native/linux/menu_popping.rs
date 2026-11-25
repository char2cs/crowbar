//! The shell's context menus: develop's GTK menu, opened over the shell window where it asks.

use std::cell::RefCell;
use std::rc::Rc;

use gtk::glib::{self, Cast as _};
use gtk::prelude::{
    CheckMenuItemExt as _, GtkMenuExt as _, GtkMenuItemExt as _, GtkWindowExt as _,
    MenuShellExt as _, WidgetExt as _,
};

use crate::backend::menu_popping::{ChosenEntry, MenuPopping};
use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::error::{Error, Result};
use crate::menu::{MenuEdge, MenuEntry, MenuRequest};
use crate::view::ParentWindow;
use wry::dpi::LogicalPosition;

type Picked = Rc<RefCell<Option<Vec<usize>>>>;

impl MenuPopping for GtkPlatform {
    fn popup_menu(
        &self,
        parent: ParentWindow,
        request: MenuRequest,
        answer: ChosenEntry,
    ) -> Result<()> {
        let ParentWindow::X11(top) = parent else {
            return Err(Error::UnsupportedParent("a Linux menu needs an X11 window"));
        };
        self.post(move |_| {
            glib::MainContext::default().spawn_local(pop(request, top, answer));
        })
    }
}

// PLATFORM: linux GTK deactivates a menu before it activates the chosen row, so the answer waits for the next turn of the loop after `deactivate`.
async fn pop(
    request: MenuRequest,
    top: u32,
    answer: ChosenEntry,
) {
    let picked = Picked::default();
    let menu = build(&request.entries, &[], &picked);
    let (closed, wait) = async_channel::bounded(1);
    menu.connect_deactivate(on_close(closed));
    let shown = show(&menu, top, &request);
    if shown && wait.recv().await.is_err() {
        tracing::debug!("context menu: gone before it closed");
    }
    answer(picked.take());
    discard(&menu);
}

fn on_close(closed: async_channel::Sender<()>) -> impl Fn(&gtk::Menu) + 'static {
    move |_| tell_closed(&closed)
}

fn tell_closed(closed: &async_channel::Sender<()>) {
    if closed.try_send(()).is_err() {
        tracing::debug!("context menu: deactivated twice");
    }
}

fn show(
    menu: &gtk::Menu,
    top: u32,
    request: &MenuRequest,
) -> bool {
    let display = menu.display();
    let Ok(display) = display.downcast::<gdkx11::X11Display>() else {
        tracing::warn!("context menu: not on an X11 display");
        return false;
    };
    let shell = gdkx11::X11Window::foreign_new_for_display(&display, top.into());
    let at = LogicalPosition::new(request.x, request.y).cast::<i32>();
    let at = gtk::gdk::Rectangle::new(at.x, at.y, 1, 1);
    let gravity = gravity_of(request.edge);
    menu.popup_at_rect(shell.upcast_ref(), &at, gravity, gravity, None);
    menu.is_visible()
}

fn gravity_of(edge: MenuEdge) -> gtk::gdk::Gravity {
    match edge {
        MenuEdge::Start => gtk::gdk::Gravity::NorthWest,
        MenuEdge::End => gtk::gdk::Gravity::NorthEast,
    }
}

fn build(
    entries: &[MenuEntry],
    at: &[usize],
    picked: &Picked,
) -> gtk::Menu {
    let menu = gtk::Menu::new();
    for (index, entry) in entries.iter().enumerate() {
        let path = [at, &[index]].concat();
        menu.append(&row(entry, path, picked));
    }
    menu.show_all();
    menu
}

fn row(
    entry: &MenuEntry,
    path: Vec<usize>,
    picked: &Picked,
) -> gtk::MenuItem {
    match entry {
        MenuEntry::Separator => gtk::SeparatorMenuItem::new().upcast(),
        MenuEntry::Item {
            label,
            enabled,
            checked,
        } => {
            let row = checked
                .then(|| checked_item(label))
                .unwrap_or_else(|| gtk::MenuItem::with_label(label));
            row.set_sensitive(*enabled);
            row.connect_activate(pick(path, picked));
            row
        },
        MenuEntry::Submenu {
            label,
            enabled,
            entries,
        } => {
            let row = gtk::MenuItem::with_label(label);
            row.set_sensitive(*enabled);
            row.set_submenu(Some(&build(entries, &path, picked)));
            row
        },
    }
}

fn pick(
    path: Vec<usize>,
    picked: &Picked,
) -> impl Fn(&gtk::MenuItem) + 'static {
    let picked = Rc::clone(picked);
    move |_| *picked.borrow_mut() = Some(path.clone())
}

fn checked_item(label: &str) -> gtk::MenuItem {
    let check = gtk::CheckMenuItem::with_label(label);
    check.set_active(true);
    check.upcast()
}

// PLATFORM: linux a popped-up menu lives in a toplevel GTK keeps; closing that window frees both.
fn discard(menu: &gtk::Menu) {
    let toplevel = menu
        .toplevel()
        .and_then(|top| top.downcast::<gtk::Window>().ok());
    if let Some(window) = toplevel {
        window.close();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const PARENTS: [(ParentWindow, usize); 2] =
        [(ParentWindow::X11(1), 1), (ParentWindow::AppKit(1), 0)];

    fn post_over(parent: ParentWindow) -> (bool, usize) {
        let (jobs, inbox) = async_channel::unbounded();
        let platform = GtkPlatform {
            jobs,
            stopped: None,
        };
        let request = MenuRequest {
            entries: vec![MenuEntry::Separator],
            x: 0.0,
            y: 0.0,
            edge: MenuEdge::Start,
        };
        let posted = platform.popup_menu(parent, request, Box::new(|_| ()));
        (posted.is_ok(), inbox.len())
    }

    #[test]
    fn only_a_menu_over_an_x11_window_is_posted_to_the_gtk_thread() {
        for (parent, jobs_posted) in PARENTS {
            let want = (jobs_posted == 1, jobs_posted);
            assert_eq!(post_over(parent), want, "{parent:?}");
        }
    }
}
