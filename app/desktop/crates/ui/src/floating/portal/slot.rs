use gpui::{App, ElementId, EntityId, Window};

pub(crate) fn keyed(
    id: impl Into<ElementId>,
    release: impl FnOnce(EntityId, &mut App) + 'static,
    (window, cx): (&mut Window, &mut App),
) -> EntityId {
    let held = window.use_keyed_state(id, cx, |_, cx| {
        let slot = cx.entity_id();
        cx.on_release(move |(), cx| release(slot, cx)).detach();
    });
    held.entity_id()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Float;
    use crate::test_support::floats::{covered, toggled};
    use crate::test_support::probe::settle;

    #[gpui::test]
    fn a_popover_stops_covering_once_its_portal_stops_rendering(cx: &mut TestAppContext) {
        let (shown, mut owner) = toggled(cx, Float::Popover);
        assert_eq!(covered(&mut owner).len(), 1, "its card covers");
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert_eq!(covered(&mut owner), Vec::new());
    }
}
