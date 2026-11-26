//! A portal's place in its owner's float windows, released with the portal's element state.

use gpui::{App, Context, Entity, EntityId, WeakEntity};

use crate::floating::layers::owner::Owner;

pub(crate) struct Slot {
    owner: WeakEntity<Owner>,
}

impl Slot {
    pub(crate) fn new(
        owner: &Entity<Owner>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let slot = cx.entity_id();
        cx.on_release(move |this: &mut Self, cx| this.hide(slot, cx))
            .detach();
        Self {
            owner: owner.downgrade(),
        }
    }

    fn hide(
        &self,
        slot: EntityId,
        cx: &mut App,
    ) {
        if let Err(error) = self.owner.update(cx, |owner, cx| owner.hide(slot, cx)) {
            tracing::debug!(%error, "the owner of a float layer closed first");
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Float;
    use crate::test_support::floats::{toggled, windows};
    use crate::test_support::probe::settle;

    #[gpui::test]
    fn a_popover_window_closes_once_its_last_portal_stops_rendering(cx: &mut TestAppContext) {
        let (shown, mut owner) = toggled(cx, Float::Popover);
        assert_eq!(windows(&mut owner), 2);
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert_eq!(windows(&mut owner), 1);
    }
}
