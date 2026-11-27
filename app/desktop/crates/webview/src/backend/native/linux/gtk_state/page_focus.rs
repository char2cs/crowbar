use gtk::gdk;
use gtk::gdk::EventMask;
use gtk::glib::prelude::ObjectExt as _;
use gtk::prelude::WidgetExt as _;
use platform::FocusChange;
use wry::WebViewExtUnix as _;

use crate::backend::native::linux::gtk_state::live_view::LiveView;

const KEYS: EventMask = EventMask::KEY_PRESS_MASK.union(EventMask::KEY_RELEASE_MASK);
pub(crate) const NOBODY: u32 = 0;
const FOCUS_IN: &str = "focus-in-event";
const FOCUS_OUT: &str = "focus-out-event";

pub(crate) fn give_keys<'a>(
    pages: impl IntoIterator<Item = &'a LiveView>,
    owner: u32,
) {
    for live in pages {
        select_keys(live, |events| keys_for(events, (live.page_window, owner)));
    }
}

pub(crate) fn follow(
    live: &LiveView,
    change: FocusChange,
) {
    let (owner, signal) = match change {
        FocusChange::Gained(window) => (window, FOCUS_IN),
        FocusChange::Lost(_) => (NOBODY, FOCUS_OUT),
    };
    give_keys([live], owner);
    show_focus(live, signal);
}

fn keys_for(
    events: EventMask,
    (page, owner): (u32, u32),
) -> EventMask {
    if page != owner {
        return events - KEYS;
    }
    events | KEYS
}

// PLATFORM: linux X hands a key to the deepest window under the pointer that selects keys while the pointer lies inside the focus window, so only the page holding the keyboard selects them, whoever gave it the keyboard.
fn select_keys(
    live: &LiveView,
    keys: impl FnOnce(EventMask) -> EventMask,
) {
    if let Some(window) = live.view.webview().window() {
        window.set_events(keys(window.events()));
    }
}

// PLATFORM: linux the page's GtkWindow wraps a foreign X window, so GDK never hears its focus change; without an active window WebKit draws no focus, caret or active selection.
fn show_focus(
    live: &LiveView,
    signal: &str,
) {
    let Some(toplevel) = live.view.webview().toplevel() else {
        return;
    };
    let event = gdk::Event::new(gdk::EventType::FocusChange);
    toplevel.emit_by_name::<bool>(signal, &[&event]);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_page_that_owns_the_keyboard_selects_keys_and_each_keeps_its_other_events() {
        let other = EventMask::BUTTON_PRESS_MASK;
        let held = keys_for(other, (1, 1));
        assert_eq!(held, other | KEYS, "the owner");
        assert_eq!(keys_for(held, (1, 2)), other, "another page owns it");
        assert_eq!(keys_for(held, (1, NOBODY)), other, "no page owns it");
    }
}
