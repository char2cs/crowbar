//! The installed float host, and the cards each float window painted in its last frame.

use std::collections::HashMap;
use std::rc::Rc;

use gpui::{App, Global, WindowId};

use crate::floating::layers::float_host::FloatHost;
use crate::floating::layers::float_region::FloatRegion;
use crate::floating::layers::part::{Cards, Part};

#[derive(Default)]
pub(crate) struct Registry {
    host: Option<Rc<dyn FloatHost>>,
    cards: HashMap<WindowId, Cards>,
}

impl Global for Registry {}

/// Installs `host`, so portals paint in float windows from now on.
pub fn install_float_host(
    host: impl FloatHost + 'static,
    cx: &mut App,
) {
    cx.default_global::<Registry>().host = Some(Rc::new(host));
}

impl Registry {
    pub(crate) fn host(cx: &App) -> Option<Rc<dyn FloatHost>> {
        cx.try_global::<Self>()?.host.clone()
    }

    pub(crate) fn is_float(
        window: WindowId,
        cx: &App,
    ) -> bool {
        cx.try_global::<Self>()
            .is_some_and(|registry| registry.cards.contains_key(&window))
    }

    pub(crate) fn start_frame(
        float: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().cards.insert(float, Vec::new());
    }

    pub(crate) fn note(
        float: WindowId,
        card: (Part, FloatRegion),
        cx: &mut App,
    ) {
        if let Some(cards) = cx.default_global::<Self>().cards.get_mut(&float) {
            cards.push(card);
        }
    }

    pub(crate) fn painted(
        float: WindowId,
        cx: &App,
    ) -> Cards {
        cx.try_global::<Self>()
            .and_then(|registry| registry.cards.get(&float).cloned())
            .unwrap_or_default()
    }

    pub(crate) fn forget_float(
        float: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().cards.remove(&float);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn a_float_window_keeps_the_cards_of_its_last_frame_until_it_is_forgotten(
        cx: &mut TestAppContext
    ) {
        let window = cx
            .add_empty_window()
            .update(|window, _| window.window_handle().window_id());
        let card = (Part::Solid, FloatRegion::default());
        cx.update(|cx| {
            Registry::note(window, card, cx);
            assert!(
                !Registry::is_float(window, cx),
                "noted before its first frame"
            );
            Registry::start_frame(window, cx);
            Registry::note(window, card, cx);
            assert_eq!(Registry::painted(window, cx), [card]);
            Registry::start_frame(window, cx);
            assert!(Registry::painted(window, cx).is_empty(), "a new frame");
            Registry::forget_float(window, cx);
            assert!(!Registry::is_float(window, cx));
        });
    }
}
