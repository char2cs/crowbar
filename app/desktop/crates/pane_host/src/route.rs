use protocol::{ExtensionId, HostId};

/// Where a `crowbar://` request goes.
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
    Daemon {
        host: HostId,
        path_and_query: String,
    },
}

const PANES_PREFIX: &str = "/panes/";

/// Splits a request URI of the form `<scheme>://<host>/<path>`; `None` for anything else.
pub(crate) fn parse(scheme: &str, uri: &str) -> Option<Route> {
    let rest = uri.strip_prefix(scheme)?.strip_prefix("://")?;
    let (authority, path_and_query) = rest
        .find('/')
        .and_then(|slash| rest.split_at_checked(slash))
        .unwrap_or((rest, "/"));
    let host = HostId::new(authority).ok()?;
    let Some(panes) = path_and_query.strip_prefix(PANES_PREFIX) else {
        return Some(Route::Daemon {
            host,
            path_and_query: path_and_query.to_owned(),
        });
    };
    let path = panes.split('?').next().unwrap_or_default();
    match path.strip_prefix("core/") {
        Some(core) => Some(Route::CoreBundle {
            path: core.to_owned(),
        }),
        None => extension_route(path),
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
#[path = "route_tests.rs"]
mod tests;
