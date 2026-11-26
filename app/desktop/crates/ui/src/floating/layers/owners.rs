//! The owner of each window that has a pop-up window, made on first use and dropped with the
//! window.

use std::collections::HashMap;

use gpui::{
    App, AppContext as _, Entity, EntityId, Global, Subscription, WeakEntity, Window, WindowId,
};

use crate::floating::layers::owner::Owner;
use crate::floating::layers::registry::Registry;

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

    pub(crate) fn hide(
        owner: &WeakEntity<Owner>,
        slot: EntityId,
        cx: &mut App,
    ) {
        if let Err(error) = owner.update(cx, |owner, cx| owner.hide(slot, cx)) {
            tracing::debug!(%error, "the owner of a passive layer closed first");
        }
    }
}

fn forget(
    closed: WindowId,
    cx: &mut App,
) {
    let Some((owner, _)) = cx.global_mut::<Owners>().owners.remove(&closed) else {
        return;
    };
    let Some(float) = owner.read(cx).float() else {
        return;
    };
    if let Err(error) = float.update(cx, |_, window, _| window.remove_window()) {
        tracing::debug!(%error, "the pop-up window was already closed");
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement as _, TestAppContext, div};

    use crate::Portal;
    use crate::test_support::floats::{floated, windows};
    use crate::test_support::probe::open_floating;
    use crate::testing::newest_window;

    #[gpui::test]
    fn a_portal_inside_a_pop_up_window_paints_in_that_window(cx: &mut TestAppContext) {
        let inner = || div().child(Portal::here("inner", |_, _| floated()));
        let mut owner = open_floating(cx, move |_, _| Portal::cover("outer", move |_, _| inner()));
        let mut top = newest_window(&mut owner);
        assert!(top.debug_bounds("floated").is_some());
        assert_eq!(windows(&mut owner), 2, "no window of its own");
    }
}
