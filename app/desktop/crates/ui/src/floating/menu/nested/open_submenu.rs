//! The nested menu a menu currently shows, with the subscriptions that tie it to its parent.

use gpui::{Entity, Subscription};

pub(crate) struct OpenSubmenu<M> {
    pub(crate) index: usize,
    pub(crate) menu: Entity<M>,
    pub(crate) _subscriptions: [Subscription; 2],
}

impl<M> std::fmt::Debug for OpenSubmenu<M> {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("OpenSubmenu")
            .field("index", &self.index)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    struct Probe;

    #[gpui::test]
    fn debug_shows_the_open_index_only(cx: &mut TestAppContext) {
        let menu = cx.new(|_| Probe);
        let open = OpenSubmenu {
            index: 3,
            menu,
            _subscriptions: [Subscription::new(|| {}), Subscription::new(|| {})],
        };
        assert_eq!(format!("{open:?}"), "OpenSubmenu { index: 3, .. }");
    }
}
