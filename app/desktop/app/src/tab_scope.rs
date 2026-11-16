const ALL: &str = "all";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum TabScope {
    /// The tabs of the active project.
    #[default]
    Project,
    /// Every tab of the window.
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
        active: Option<&str>,
        projects: &[String],
    ) -> bool {
        let Some(active) = active.filter(|_| self == Self::Project) else {
            return true;
        };
        projects.is_empty() || projects.iter().any(|project| project == active)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (
        TabScope,
        Option<&'static str>,
        &'static [&'static str],
        bool,
    );

    const CASES: &[Case] = &[
        (TabScope::Project, Some("p1"), &["p1"], true),
        (TabScope::Project, Some("p1"), &["p2"], false),
        (TabScope::Project, Some("p1"), &["p2", "p1"], true),
        (TabScope::Project, Some("p1"), &[], true),
        (TabScope::Project, None, &["p2"], true),
        (TabScope::All, Some("p1"), &["p2"], true),
    ];

    #[test]
    fn the_project_scope_lists_its_own_and_unowned_tabs_and_the_all_scope_lists_everything() {
        for (scope, active, projects, listed) in CASES {
            let owned: Vec<String> = projects.iter().map(|p| (*p).to_owned()).collect();
            assert_eq!(
                scope.lists(*active, &owned),
                *listed,
                "{scope:?} {active:?} {projects:?}"
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
