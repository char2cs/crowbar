//! The owner of each window that has float windows, made on first use.

use std::collections::HashMap;

use gpui::{AnyWindowHandle, App, AppContext as _, Entity, Global, Subscription, Window, WindowId};

use crate::floating::layers::owner::Owner;
use crate::floating::layers::registry::Registry;
use crate::floating::portal::float::Float;

#[derive(Default)]
pub(crate) struct Owners {
    owners: HashMap<WindowId, (Entity<Owner>, Subscription)>,
}

impl Global for Owners {}

impl Owners {
    pub(crate) fn of(
        window: &mut Window,
        cx: &mut App,
    ) -> Option<Entity<Owner>> {
        let id = window.window_handle().window_id();
        if Registry::host(cx).is_none() || Registry::is_float(id, cx) {
            return None;
        }
        let known = cx.default_global::<Self>().owners.get(&id);
        if let Some((owner, _)) = known {
            return Some(owner.clone());
        }
        let owner = cx.new(|cx| Owner::new(window, cx));
        let closed = cx.on_window_closed(move |cx, closed| forget(closed, cx));
        let entry = (owner.clone(), closed);
        cx.global_mut::<Self>().owners.insert(id, entry);
        Some(owner)
    }
}

/// Whether `window` has a float window open that holds its keyboard: a dialog or a menu.
pub fn holds_keys(
    window: &Window,
    cx: &App,
) -> bool {
    let id = window.window_handle().window_id();
    cx.try_global::<Owners>()
        .and_then(|owners| owners.owners.get(&id))
        .is_some_and(|(owner, _)| owner.read(cx).holds_keys().is_some())
}

/// The float window of kind `float` that `window` has open, if any.
pub fn open_float_window(
    window: &Window,
    float: Float,
    cx: &App,
) -> Option<AnyWindowHandle> {
    let id = window.window_handle().window_id();
    let (owner, _) = cx.try_global::<Owners>()?.owners.get(&id)?;
    let floats = owner.read(cx).floats();
    floats
        .into_iter()
        .find(|(kind, _)| *kind == float)
        .map(|(_, open)| open)
}

fn forget(
    closed: WindowId,
    cx: &mut App,
) {
    let Some((owner, _)) = cx.global_mut::<Owners>().owners.remove(&closed) else {
        return;
    };
    owner.update(cx, |owner, cx| owner.close_floats(cx));
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement as _, TestAppContext, div};

    use super::holds_keys;
    use crate::test_support::floats::{floated, toggled, windows};
    use crate::test_support::probe::open_floating;
    use crate::testing::newest_window;
    use crate::{Float, Portal};

    #[gpui::test]
    fn a_portal_inside_a_float_window_paints_in_that_window(cx: &mut TestAppContext) {
        let inner = || div().child(Portal::here("inner", |_, _| floated()));
        let mut owner = open_floating(cx, move |_, _| Portal::cover("outer", move |_, _| inner()));
        let mut top = newest_window(&mut owner);
        assert!(top.debug_bounds("floated").is_some());
        assert_eq!(windows(&mut owner), 2, "no window of its own");
    }

    #[gpui::test]
    fn an_owner_activated_under_its_open_menu_hands_the_activation_to_the_menu(
        cx: &mut TestAppContext
    ) {
        let (_, mut owner) = toggled(cx, Float::Popover);
        let mut menu = newest_window(&mut owner);
        assert!(
            owner.update(|window, cx| holds_keys(window, cx)),
            "the menu holds the keys"
        );
        owner.update(|window, _| window.activate_window());
        owner.run_until_parked();
        assert!(
            menu.update(|window, _| window.is_window_active()),
            "as a window manager does"
        );
    }
}
