use http::{HeaderMap, HeaderName, HeaderValue};
use protocol::PaneOrigin;

use crate::error::Error;

const API_PREFIX: &str = "/v0";
const EXTENSION_ROUTES: &str = "/v0/ext/";

/// Header the transport sets on every extension-scoped request, HTTP and WebSocket alike, and
/// strips from every other, so a daemon can apply its own per-origin check without trusting the
/// page. Its value is the extension id.
pub const EXTENSION_HEADER: &str = "x-crowbar-extension";

pub(crate) fn check(origin: &PaneOrigin, path_and_query: &str) -> Result<(), Error> {
    let path = path_and_query.split(['?', '#']).next().unwrap_or_default();
    if is_plain_path(path) && allows(origin, path) {
        return Ok(());
    }
    Err(Error::Forbidden(path_and_query.to_owned()))
}

pub(crate) fn stamp(headers: &mut HeaderMap, origin: &PaneOrigin) {
    let name = HeaderName::from_static(EXTENSION_HEADER);
    headers.remove(&name);
    let value = origin
        .extension()
        .and_then(|id| HeaderValue::from_str(id.as_str()).ok());
    if let Some(value) = value {
        headers.insert(name, value);
    }
}

fn allows(origin: &PaneOrigin, path: &str) -> bool {
    match origin {
        PaneOrigin::Core => path == API_PREFIX || path.starts_with("/v0/"),
        PaneOrigin::Extension(id) => path
            .strip_prefix(EXTENSION_ROUTES)
            .and_then(|rest| rest.strip_prefix(id.as_str()))
            .is_some_and(|rest| rest.is_empty() || rest.starts_with('/')),
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
