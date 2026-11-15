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
const RELAY_PATH: &str = "/__crowbar/relay";

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
    let Some(panes) = path_and_query.strip_prefix(PANES_PREFIX) else {
        return Some(daemon_route(host, path_and_query));
    };
    let path = panes.split('?').next().unwrap_or_default();
    match path.strip_prefix("core/") {
        Some(core) => Some(Route::CoreBundle {
            path: core.to_owned(),
        }),
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

    fn route_cases() -> [Case; 5] {
        let git = ExtensionId::new("git").unwrap();
        [
            (
                "crowbar://local/panes/core/chat/index.html",
                Some(Route::CoreBundle {
                    path: "chat/index.html".into(),
                }),
            ),
            (
                "crowbar://local/panes/core/",
                Some(Route::CoreBundle {
                    path: String::new(),
                }),
            ),
            (
                "crowbar://local/panes/ext/git/review/index.html?x=1",
                Some(Route::ExtensionBundle {
                    extension: git,
                    path: "review/index.html".into(),
                }),
            ),
            (
                "crowbar://paired-1/v0/threads?limit=5",
                Some(Route::Daemon {
                    host: HostId::new("paired-1").unwrap(),
                    path_and_query: "/v0/threads?limit=5".into(),
                }),
            ),
            (
                "crowbar://local",
                Some(Route::Daemon {
                    host: HostId::new("local").unwrap(),
                    path_and_query: "/".into(),
                }),
            ),
        ]
    }

    #[test]
    fn bundle_and_daemon_routes() {
        for (uri, expected) in route_cases() {
            assert_eq!(parse("crowbar", uri), expected, "{uri}");
        }
    }

    #[test]
    fn malformed_uris_have_no_route() {
        for uri in [
            "http://local/v0/x",
            "crowbar:/local/x",
            "crowbar://BAD HOST/x",
            "crowbar://local/panes/ext/Bad/x",
            "crowbar://local/panes/ext/git",
            "crowbar://local/panes/unknown/x",
        ] {
            assert_eq!(parse("crowbar", uri), None, "{uri}");
        }
    }

    #[test]
    fn the_relay_route_is_recognised() {
        assert_eq!(
            parse("crowbar", "crowbar://local/__crowbar/relay"),
            Some(Route::Relay {
                host: HostId::new("local").unwrap()
            }),
            "relay"
        );
    }
}
