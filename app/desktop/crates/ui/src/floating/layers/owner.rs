//! The pop-up window of one owner window: the layers it shows, and following the owner.

use gpui::{AnyWindowHandle, Context, EntityId, Subscription, Window};
use raw_window_handle::RawWindowHandle;

use crate::floating::layers::float_host::raw_handle::raw_handle;
use crate::floating::layers::registry::Registry;
use crate::floating::layers::stack::Stack;
use crate::floating::portal::layer::Layer;

pub(crate) struct Owner {
    window: AnyWindowHandle,
    raw: Option<RawWindowHandle>,
    stack: Stack,
    _watching: Vec<Subscription>,
}

impl Owner {
    pub(crate) fn new(
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let watching = vec![
            cx.observe_window_bounds(window, |this: &mut Self, _, cx| this.cover(cx)),
            cx.observe_window_activation(window, |_, _, cx| cx.notify()),
        ];
        Self {
            window: Window::window_handle(window),
            raw: raw_handle(window),
            stack: Stack::default(),
            _watching: watching,
        }
    }

    pub(crate) fn place(
        &mut self,
        slot: EntityId,
        layer: Layer,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let opens = self.stack.place(slot, layer);
        cx.notify();
        opens
    }

    pub(crate) fn hide(
        &mut self,
        slot: EntityId,
        cx: &mut Context<'_, Self>,
    ) {
        self.stack.hide(slot);
        cx.notify();
    }

    pub(crate) fn shown(&self) -> Vec<Layer> {
        self.stack.shown()
    }

    pub(crate) fn window(&self) -> AnyWindowHandle {
        self.window
    }

    pub(crate) fn raw(&self) -> Option<RawWindowHandle> {
        self.raw
    }

    pub(crate) fn float(&self) -> Option<AnyWindowHandle> {
        self.stack.window()
    }

    pub(crate) fn set_window(
        &mut self,
        window: Option<AnyWindowHandle>,
    ) {
        self.stack.set_window(window);
    }

    fn cover(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(host), Some(owner), Some(float)) = (Registry::host(cx), self.raw, self.float())
        else {
            return;
        };
        let raw = float
            .update(cx, |_, window, _| raw_handle(window))
            .ok()
            .flatten();
        if let Some(raw) = raw {
            host.cover(raw, owner);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Float;
    use crate::test_support::floats::{toggled, windows};

    #[gpui::test]
    fn the_pop_up_window_closes_with_its_owner(cx: &mut TestAppContext) {
        let (_, mut owner) = toggled(cx, Float::Passive);
        assert_eq!(windows(&mut owner), 2);
        owner.update(|window, _| window.remove_window());
        owner.run_until_parked();
        assert!(cx.update(|cx| cx.windows().is_empty()));
    }
}
