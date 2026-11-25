use sidebar::ScopeSync;

use crate::shell::shell_view::events::first_run::hosts::Hosts;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Landing {
    Home,
    FirstRun,
    AddHost,
}

pub(crate) fn landing_for(
    spaces: usize,
    sync: ScopeSync,
    hosts: &Hosts,
) -> Landing {
    if spaces > 0 {
        return Landing::Home;
    }
    if hosts.has_local {
        return match sync {
            ScopeSync::Ready => Landing::FirstRun,
            ScopeSync::Pending => Landing::Home,
        };
    }
    if hosts.paired == 0 {
        return Landing::AddHost;
    }
    Landing::Home
}

pub(crate) fn space_name(folder: &str) -> Option<String> {
    folder
        .trim_end_matches(['/', '\\'])
        .rsplit(['/', '\\'])
        .next()
        .filter(|name| !name.is_empty())
        .map(str::to_owned)
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Case {
        spaces: usize,
        sync: ScopeSync,
        hosts: Hosts,
        expected: Landing,
    }

    const LOCAL_AND_ONE: Hosts = Hosts {
        has_local: true,
        paired: 1,
    };
    const LOCAL_ALONE: Hosts = Hosts {
        has_local: true,
        paired: 0,
    };
    const ONE_PAIRED: Hosts = Hosts {
        has_local: false,
        paired: 1,
    };
    const NONE: Hosts = Hosts {
        has_local: false,
        paired: 0,
    };

    const fn case(
        spaces: usize,
        sync: ScopeSync,
        hosts: Hosts,
        expected: Landing,
    ) -> Case {
        Case {
            spaces,
            sync,
            hosts,
            expected,
        }
    }

    const LANDINGS: &[Case] = &[
        case(2, ScopeSync::Ready, LOCAL_AND_ONE, Landing::Home),
        case(0, ScopeSync::Pending, LOCAL_AND_ONE, Landing::Home),
        case(0, ScopeSync::Ready, LOCAL_AND_ONE, Landing::FirstRun),
        case(0, ScopeSync::Ready, LOCAL_ALONE, Landing::FirstRun),
        case(0, ScopeSync::Pending, NONE, Landing::AddHost),
        case(0, ScopeSync::Ready, NONE, Landing::AddHost),
        case(0, ScopeSync::Ready, ONE_PAIRED, Landing::Home),
    ];

    #[test]
    fn the_landing_follows_spaces_then_the_local_daemon_then_paired_hosts() {
        for case in LANDINGS {
            let landing = landing_for(case.spaces, case.sync, &case.hosts);
            assert_eq!(landing, case.expected, "{:?}", case.hosts);
        }
    }

    #[test]
    fn a_space_is_named_after_its_folder() {
        const CASES: &[(&str, Option<&str>)] = &[
            ("/home/me/code", Some("code")),
            ("/home/me/code/", Some("code")),
            ("C:\\work\\app", Some("app")),
            ("/", None),
            ("", None),
        ];
        for &(folder, expected) in CASES {
            assert_eq!(space_name(folder).as_deref(), expected, "{folder:?}");
        }
    }
}
