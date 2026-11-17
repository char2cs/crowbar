use protocol::{ExtensionId, HostId};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Route {
    /// A file of the core pane bundle.
    CoreBundle { path: String },
    /// A file of an extension's pane bundle.
    ExtensionBundle {
        extension: ExtensionId,
        path: String,
    },
    /// The daemon of `host`.
    Relay { host: HostId },
    Daemon {
        host: HostId,
        path_and_query: String,
    },
}

const PANES_PREFIX: &str = "/panes/";
const FONTS_PREFIX: &str = "/fonts/";
const RELAY_PATH: &str = "/__crowbar/relay";

fn core_bundle(path: &str) -> Route {
    Route::CoreBundle {
        path: path.to_owned(),
    }
}

pub(crate) fn parse(
    scheme: &str,
    uri: &str,
) -> Option<Route> {
    let rest = uri.strip_prefix(scheme)?.strip_prefix("://")?;
    let (authority, path_and_query) = rest
        .find('/')
        .and_then(|slash| rest.split_at_checked(slash))
        .unwrap_or((rest, "/"));
    let host = HostId::new(authority).ok()?;
    if path_and_query == RELAY_PATH {
        return Some(Route::Relay { host });
    }
    if path_and_query.starts_with(FONTS_PREFIX) {
        return Some(core_bundle(path_and_query.trim_start_matches('/')));
    }
    let Some(panes) = path_and_query.strip_prefix(PANES_PREFIX) else {
        return Some(daemon_route(host, path_and_query));
    };
    let path = panes.split('?').next().unwrap_or_default();
    match path.strip_prefix("core/") {
        Some(core) => Some(core_bundle(core)),
        None => extension_route(path),
    }
}

fn daemon_route(
    host: HostId,
    path_and_query: &str,
) -> Route {
    Route::Daemon {
        host,
        path_and_query: path_and_query.to_owned(),
    }
}

fn extension_route(path: &str) -> Option<Route> {
    let (name, file) = path.strip_prefix("ext/")?.split_once('/')?;
    Some(Route::ExtensionBundle {
        extension: ExtensionId::new(name).ok()?,
        path: file.to_owned(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static str, Option<Route>);

    fn core(path: &str) -> Route {
        Route::CoreBundle { path: path.into() }
    }

    fn daemon(
        host: &str,
        path_and_query: &str,
    ) -> Route {
        Route::Daemon {
            host: HostId::new(host).unwrap(),
            path_and_query: path_and_query.into(),
        }
    }

    #[test]
    fn uris_resolve_to_their_route_or_to_none() {
        let git = ExtensionId::new("git").unwrap();
        let cases: Vec<Case> = vec![
            (
                "crowbar://local/panes/core/chat/index.html",
                Some(core("chat/index.html")),
            ),
            (
                "crowbar://local/fonts/CalSansUI.woff2",
                Some(core("fonts/CalSansUI.woff2")),
            ),
            ("crowbar://local/panes/core/", Some(core(""))),
            (
                "crowbar://local/panes/ext/git/review/index.html?x=1",
                Some(Route::ExtensionBundle {
                    extension: git,
                    path: "review/index.html".into(),
                }),
            ),
            (
                "crowbar://paired-1/v0/threads?limit=5",
                Some(daemon("paired-1", "/v0/threads?limit=5")),
            ),
            ("crowbar://local", Some(daemon("local", "/"))),
            (
                "crowbar://local/__crowbar/relay",
                Some(Route::Relay {
                    host: HostId::new("local").unwrap(),
                }),
            ),
        ];
        for (uri, expected) in cases {
            assert_eq!(parse("crowbar", uri), expected, "{uri}");
        }
    }

    #[test]
    fn malformed_uris_have_no_route() {
        let cases = vec![
            "http://local/v0/x",
            "crowbar:/local/x",
            "crowbar://BAD HOST/x",
            "crowbar://local/panes/ext/Bad/x",
            "crowbar://local/panes/ext/git",
            "crowbar://local/panes/unknown/x",
        ];
        for uri in cases {
            assert_eq!(parse("crowbar", uri), None, "{uri}");
        }
    }
}
