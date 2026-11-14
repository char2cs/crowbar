use protocol::{ExtensionId, PaneKind};

use crate::error::Error;

const API_PREFIX: &str = "/v0";
const EXTENSION_ROUTES: &str = "/v0/ext/";

/// Header the transport sets on every extension-scoped request and strips from every other, so a
/// daemon can apply its own per-origin check without trusting the page.
pub const EXTENSION_HEADER: &str = "x-crowbar-extension";

/// What a pane origin may reach on a daemon: core panes the whole `/v0` API, an extension only
/// its own `/v0/ext/<id>/...` routes, which hold its calls and its own blobs. The daemon
/// enforces the same rule independently.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum AccessScope {
    /// A built-in pane.
    Core,
    /// A pane provided by the named extension.
    Extension(ExtensionId),
}

impl AccessScope {
    /// The scope of a pane of `kind`.
    ///
    /// # Errors
    /// [`Error::BadGrant`] when an extension pane names no extension or a core pane names one.
    pub fn of(kind: PaneKind, extension: Option<&ExtensionId>) -> Result<Self, Error> {
        match (kind, extension) {
            (PaneKind::Core, None) => Ok(Self::Core),
            (PaneKind::Extension, Some(id)) => Ok(Self::Extension(id.clone())),
            (PaneKind::Core, Some(_)) => Err(Error::BadGrant("a core pane has no extension")),
            (PaneKind::Extension, None) => {
                Err(Error::BadGrant("an extension pane needs its extension"))
            },
        }
    }

    /// Checks the request target (path and optional query) against this scope.
    ///
    /// # Errors
    /// [`Error::Forbidden`] for any path outside the scope or that a parser could read two ways.
    pub fn check(&self, path_and_query: &str) -> Result<(), Error> {
        let path = path_and_query.split(['?', '#']).next().unwrap_or_default();
        if is_plain_path(path) && self.allows(path) {
            return Ok(());
        }
        Err(Error::Forbidden(path_and_query.to_owned()))
    }

    pub(crate) fn extension(&self) -> Option<&ExtensionId> {
        match self {
            Self::Extension(id) => Some(id),
            Self::Core => None,
        }
    }

    fn allows(&self, path: &str) -> bool {
        match self {
            Self::Core => path == API_PREFIX || path.starts_with("/v0/"),
            Self::Extension(id) => path
                .strip_prefix(EXTENSION_ROUTES)
                .and_then(|rest| rest.strip_prefix(id.as_str()))
                .is_some_and(|rest| rest.is_empty() || rest.starts_with('/')),
        }
    }
}

fn is_plain_path(path: &str) -> bool {
    let control = |b: u8| b < 0x20 || b == 0x7f || b == b'\\';
    if !path.starts_with('/') || path.contains("//") || path.bytes().any(control) {
        return false;
    }
    let lowered = path.to_ascii_lowercase();
    if ["%2e", "%2f", "%5c", "%00"]
        .iter()
        .any(|bad| lowered.contains(bad))
    {
        return false;
    }
    path.split('/')
        .all(|segment| segment != "." && segment != "..")
}

#[cfg(test)]
#[path = "access_scope_tests.rs"]
mod tests;
