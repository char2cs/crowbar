/// A command line tool the first-run system check looks for.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub enum Tool {
    /// Git, which Crowbar cannot work without.
    Git,
    /// The GitHub CLI, which only adds GitHub features.
    Gh,
    /// The GitLab CLI, which only adds GitLab features.
    Glab,
}

impl Tool {
    /// Every tool, in the order the check lists them.
    pub const ALL: [Self; 3] = [Self::Git, Self::Gh, Self::Glab];

    pub(crate) fn route(self) -> &'static str {
        match self {
            Self::Git => "/v0/system/prerequisites/git",
            Self::Gh => "/v0/system/prerequisites/gh",
            Self::Glab => "/v0/system/prerequisites/glab",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_tool_has_its_own_probe_route() {
        let routes: Vec<_> = Tool::ALL.iter().map(|tool| tool.route()).collect();
        assert_eq!(
            routes,
            [
                "/v0/system/prerequisites/git",
                "/v0/system/prerequisites/gh",
                "/v0/system/prerequisites/glab",
            ]
        );
    }
}
