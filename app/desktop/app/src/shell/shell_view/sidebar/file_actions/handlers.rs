use gpui::{Context, Window};

pub(super) type Then<V, T> = Box<dyn FnOnce(&mut V, T, &mut Window, &mut Context<'_, V>)>;
pub(super) type Failed<V> = fn(&mut V, String, &mut Context<'_, V>);

pub(super) struct Handlers<V, T> {
    pub(super) then: Then<V, T>,
    pub(super) failed: Failed<V>,
}
