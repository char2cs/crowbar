//! The top layer of each owner window.

use std::collections::HashMap;
use std::rc::Rc;

use gpui::{App, Global, Window, WindowId};

use crate::floating::top_layer::layers::Layers;

#[derive(Default)]
pub(crate) struct Registry {
    owners: HashMap<WindowId, Rc<Layers>>,
}

impl Global for Registry {}

impl Registry {
    pub(crate) fn of(
        window: &Window,
        cx: &App,
    ) -> Option<Rc<Layers>> {
        let registry = cx.try_global::<Self>()?;
        let layers = registry.owners.get(&window.window_handle().window_id())?;
        Some(Rc::clone(layers))
    }

    pub(crate) fn insert(
        owner: WindowId,
        layers: Rc<Layers>,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().owners.insert(owner, layers);
    }

    pub(crate) fn remove(
        owner: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().owners.remove(&owner);
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, div};

    use super::*;
    use crate::test_support::probe::open_floating;

    #[gpui::test]
    fn only_an_owner_window_has_a_top_layer(cx: &mut TestAppContext) {
        let (mut owner, mut top) = open_floating(cx, |_, _| div());
        assert!(owner.update(|window, cx| Registry::of(window, cx).is_some()));
        assert!(top.update(|window, cx| Registry::of(window, cx).is_none()));
    }
}
