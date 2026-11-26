//! The installed float host, and the cards each pop-up window painted in its last frame.

use std::collections::HashMap;
use std::rc::Rc;

use gpui::{App, Global, Window, WindowId};
use raw_window_handle::{HasWindowHandle, RawWindowHandle};

use crate::floating::layers::float_host::FloatHost;
use crate::floating::layers::painted::Painted;

#[derive(Default)]
pub(crate) struct Registry {
    host: Option<Rc<dyn FloatHost>>,
    painted: HashMap<WindowId, Painted>,
}

impl Global for Registry {}

/// Installs `host`, so passive portals paint in pop-up windows from now on.
pub fn install_float_host(
    host: impl FloatHost + 'static,
    cx: &mut App,
) {
    cx.default_global::<Registry>().host = Some(Rc::new(host));
}

pub(crate) fn raw_handle(window: &Window) -> Option<RawWindowHandle> {
    HasWindowHandle::window_handle(window)
        .ok()
        .map(|handle| handle.as_raw())
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
            .is_some_and(|registry| registry.painted.contains_key(&window))
    }

    pub(crate) fn start_frame(
        float: WindowId,
        cx: &mut App,
    ) {
        let painted = &mut cx.default_global::<Self>().painted;
        painted.insert(float, Painted::default());
    }

    pub(crate) fn painting(
        float: WindowId,
        cx: &mut App,
    ) -> Option<&mut Painted> {
        cx.default_global::<Self>().painted.get_mut(&float)
    }

    pub(crate) fn painted(
        float: WindowId,
        cx: &App,
    ) -> Painted {
        cx.try_global::<Self>()
            .and_then(|registry| registry.painted.get(&float).cloned())
            .unwrap_or_default()
    }

    pub(crate) fn forget_float(
        float: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().painted.remove(&float);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::{RawWindowHandle, raw_handle};

    #[gpui::test]
    fn a_test_window_has_no_handle_so_no_host_touches_it(cx: &mut TestAppContext) {
        let window = cx.add_empty_window();
        let raw = window.update(|window, _| raw_handle(window));
        assert_eq!(raw, None::<RawWindowHandle>);
    }
}
