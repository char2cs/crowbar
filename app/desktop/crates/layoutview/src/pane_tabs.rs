//! How a pane takes part in a tab strip.

use layout::PaneInstance;

use ideshell::TabLook;

/// How a pane takes part in a tab strip.
pub trait PaneTabs {
    /// How the pane's tab looks in a strip.
    fn tab_look(
        &self,
        instance: &PaneInstance,
    ) -> TabLook {
        TabLook::plain(instance.type_id.to_string())
    }

    /// Whether the pane may be a tab of a strip. A chat never is: it only splits.
    fn stacks(
        &self,
        _instance: &PaneInstance,
    ) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::chat;

    struct Plain;

    impl PaneTabs for Plain {}

    #[test]
    fn a_tab_is_plain_and_never_stacks_unless_the_source_says_otherwise() {
        let instance = chat("a");
        assert_eq!(
            Plain.tab_look(&instance),
            TabLook::plain(instance.type_id.to_string())
        );
        assert!(!Plain.stacks(&instance));
    }
}
