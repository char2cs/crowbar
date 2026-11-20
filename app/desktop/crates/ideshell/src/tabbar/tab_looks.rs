//! How each pane's tab looks in a tab bar.

use layout::PaneInstance;

use crate::tabbar::tab_look::TabLook;

/// Gives the tab bar the look of each pane's tab; the shell implements it from the pane's title,
/// type and unsaved work.
pub trait TabLooks {
    /// How the pane's tab looks in a tab bar.
    fn tab_look(
        &self,
        instance: &PaneInstance,
    ) -> TabLook {
        TabLook::plain(instance.type_id.to_string())
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;

    struct Plain;

    impl TabLooks for Plain {}

    #[test]
    fn a_tab_is_named_after_its_pane_type_unless_the_shell_says_otherwise() {
        let instance = PaneInstance {
            pane_id: PaneId::new("a").expect("valid id"),
            type_id: PaneTypeId::new("core.terminal").expect("valid type"),
            params: Params::default(),
            state: PaneState::default(),
        };
        assert_eq!(Plain.tab_look(&instance), TabLook::plain("core.terminal"));
    }
}
