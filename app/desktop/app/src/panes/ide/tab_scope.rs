const ALL: &str = "all";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum TabScope {
    #[default]
    Project,
    All,
}

impl TabScope {
    pub(crate) fn from_setting(text: Option<&str>) -> Self {
        match text {
            Some(ALL) => Self::All,
            _ => Self::Project,
        }
    }

    pub(crate) fn lists(
        self,
        active: &str,
        space: &str,
    ) -> bool {
        self == Self::All || active.is_empty() || space.is_empty() || space == active
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (TabScope, &'static str, &'static str, bool);

    const CASES: &[Case] = &[
        (TabScope::Project, "p1", "p1", true),
        (TabScope::Project, "p1", "p2", false),
        (TabScope::Project, "p1", "", true),
        (TabScope::Project, "", "p2", true),
        (TabScope::All, "p1", "p2", true),
    ];

    #[test]
    fn the_project_scope_lists_its_own_and_unowned_tabs_and_the_all_scope_lists_everything() {
        for (scope, active, space, listed) in CASES {
            assert_eq!(
                scope.lists(active, space),
                *listed,
                "{scope:?} {active:?} {space:?}"
            );
        }
    }

    #[test]
    fn a_stored_all_picks_the_all_scope_and_anything_else_the_project_one() {
        assert_eq!(TabScope::from_setting(Some("all")), TabScope::All);
        assert_eq!(TabScope::from_setting(Some("project")), TabScope::Project);
        assert_eq!(TabScope::from_setting(Some("x")), TabScope::Project);
        assert_eq!(TabScope::from_setting(None), TabScope::Project);
    }
}
