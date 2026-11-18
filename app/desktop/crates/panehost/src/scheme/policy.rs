use protocol::PaneOrigin;

use crate::scheme::route::Route;

pub(crate) fn allows(
    origin: &PaneOrigin,
    route: &Route,
) -> bool {
    match (origin, route) {
        (_, Route::Relay { .. })
        | (PaneOrigin::Core, Route::CoreBundle { .. } | Route::Daemon { .. }) => true,
        (PaneOrigin::Extension(own), Route::ExtensionBundle { extension, .. }) => own == extension,
        (PaneOrigin::Extension(own), Route::Daemon { path_and_query, .. }) => {
            is_plain(path_and_query) && path_and_query.starts_with(&format!("/v0/ext/{own}/"))
        },
        (PaneOrigin::Core, Route::ExtensionBundle { .. })
        | (PaneOrigin::Extension(_), Route::CoreBundle { .. }) => false,
    }
}

const TRAVERSAL_MARKERS: [&str; 5] = ["..", "%2e", "%2f", "%5c", "\\"];

fn is_plain(path_and_query: &str) -> bool {
    let lowered = path_and_query.to_ascii_lowercase();
    TRAVERSAL_MARKERS
        .iter()
        .all(|marker| !lowered.contains(marker))
}

#[cfg(test)]
mod tests {
    use protocol::{ExtensionId, HostId};

    use super::*;

    type Case = (&'static str, PaneOrigin, Route, bool);

    fn extension(name: &str) -> PaneOrigin {
        PaneOrigin::Extension(ExtensionId::new(name).unwrap())
    }

    fn daemon(path: &str) -> Route {
        Route::Daemon {
            host: HostId::new("local").unwrap(),
            path_and_query: path.into(),
        }
    }

    fn bundle(name: &str) -> Route {
        Route::ExtensionBundle {
            extension: ExtensionId::new(name).unwrap(),
            path: "a".into(),
        }
    }

    fn core_bundle() -> Route {
        Route::CoreBundle {
            path: String::new(),
        }
    }

    fn relay() -> Route {
        Route::Relay {
            host: HostId::new("local").unwrap(),
        }
    }

    fn core_cases() -> Vec<Case> {
        vec![
            (
                "core reaches the core bundle",
                PaneOrigin::Core,
                core_bundle(),
                true,
            ),
            (
                "core reaches the whole daemon",
                PaneOrigin::Core,
                daemon("/v0/threads"),
                true,
            ),
            (
                "core cannot read an extension bundle",
                PaneOrigin::Core,
                bundle("git"),
                false,
            ),
            (
                "core may ask for its relay",
                PaneOrigin::Core,
                relay(),
                true,
            ),
            (
                "extension may ask for its relay",
                extension("git"),
                relay(),
                true,
            ),
        ]
    }

    fn extension_bundle_cases() -> Vec<Case> {
        vec![
            (
                "extension reaches its own bundle",
                extension("git"),
                bundle("git"),
                true,
            ),
            (
                "extension cannot read a foreign bundle",
                extension("git"),
                bundle("draw"),
                false,
            ),
            (
                "extension cannot read the core bundle",
                extension("git"),
                core_bundle(),
                false,
            ),
        ]
    }

    fn extension_route_cases() -> Vec<Case> {
        vec![
            (
                "extension reaches its own routes",
                extension("git"),
                daemon("/v0/ext/git/review.diff"),
                true,
            ),
            (
                "extension cannot use a prefix lookalike",
                extension("git"),
                daemon("/v0/ext/gitx/x"),
                false,
            ),
            (
                "extension cannot reach foreign routes",
                extension("git"),
                daemon("/v0/ext/draw/x"),
                false,
            ),
            (
                "extension cannot reach the general api",
                extension("git"),
                daemon("/v0/threads"),
                false,
            ),
        ]
    }

    fn escape_cases() -> Vec<Case> {
        vec![
            (
                "extension cannot climb with dots",
                extension("git"),
                daemon("/v0/ext/git/../other/x"),
                false,
            ),
            (
                "extension cannot climb with encoded dots",
                extension("git"),
                daemon("/v0/ext/git/%2e%2e/x"),
                false,
            ),
            (
                "extension cannot climb with upper-case encoding",
                extension("git"),
                daemon("/v0/ext/git/%2E%2E/x"),
                false,
            ),
            (
                "extension cannot smuggle an encoded slash",
                extension("git"),
                daemon("/v0/ext/git/a%2fb"),
                false,
            ),
        ]
    }

    fn cases() -> Vec<Case> {
        [
            core_cases(),
            extension_bundle_cases(),
            extension_route_cases(),
            escape_cases(),
        ]
        .concat()
    }

    #[test]
    fn every_origin_reaches_exactly_what_it_owns() {
        let failed: Vec<&str> = cases()
            .iter()
            .filter(|(_, origin, route, expected)| allows(origin, route) != *expected)
            .map(|(name, ..)| *name)
            .collect();
        assert!(failed.is_empty(), "failed cases: {failed:?}");
    }
}
