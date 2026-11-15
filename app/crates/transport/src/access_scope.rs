use http::{HeaderMap, HeaderName, HeaderValue};
use protocol::PaneOrigin;

use crate::error::Error;

const API_PREFIX: &str = "/v0";
const EXTENSION_ROUTES: &str = "/v0/ext/";

/// Header the transport sets on every extension-scoped request, HTTP and WebSocket alike, and
/// strips from every other, so a daemon can apply its own per-origin check without trusting the
/// page. Its value is the extension id.
pub const EXTENSION_HEADER: &str = "x-crowbar-extension";

pub(crate) fn check(
    origin: &PaneOrigin,
    path_and_query: &str,
) -> Result<(), Error> {
    let path = path_and_query.split(['?', '#']).next().unwrap_or_default();
    if is_plain_path(path) && allows(origin, path) {
        return Ok(());
    }
    Err(Error::Forbidden(path_and_query.to_owned()))
}

pub(crate) fn stamp(
    headers: &mut HeaderMap,
    origin: &PaneOrigin,
) {
    let name = HeaderName::from_static(EXTENSION_HEADER);
    headers.remove(&name);
    let value = origin
        .extension()
        .and_then(|id| HeaderValue::from_str(id.as_str()).ok());
    if let Some(value) = value {
        headers.insert(name, value);
    }
}

fn allows(
    origin: &PaneOrigin,
    path: &str,
) -> bool {
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
mod tests {
    use protocol::ExtensionId;

    use super::*;

    fn ext(id: &str) -> PaneOrigin {
        PaneOrigin::Extension(ExtensionId::new(id).unwrap())
    }

    #[test]
    fn core_reaches_the_api_and_nothing_else() {
        for ok in ["/v0", "/v0/projects?x=1", "/v0/ext/git/call"] {
            assert!(check(&PaneOrigin::Core, ok).is_ok(), "{ok}");
        }
        for bad in [
            "/",
            "/debug/pprof/goroutine",
            "/v00",
            "v0/x",
            "/v0/../debug",
            "/v0/%2e%2e/x",
        ] {
            assert!(check(&PaneOrigin::Core, bad).is_err(), "{bad}");
        }
    }

    #[test]
    fn an_extension_reaches_only_its_own_routes_and_blobs() {
        let scope = ext("acme-git");
        for ok in [
            "/v0/ext/acme-git",
            "/v0/ext/acme-git/call",
            "/v0/ext/acme-git/blobs/abc?x=1",
        ] {
            assert!(check(&scope, ok).is_ok(), "{ok}");
        }
        for bad in [
            "/v0/projects",
            "/v0/ext/acme-gitx/call",
            "/v0/ext/acme/call",
            "/v0/ext/other/blobs/abc",
            "/v0/ext/",
            "/v0/ext//acme-git/call",
            "/v0/ext/acme-git/../other/call",
            "/v0/ext/acme-git/%2E%2E/other",
            "/v0/ext/acme-git/..%2fother",
            "/v0/ext/acme-git\\..\\other",
            "/v0/ext/acme-git/a\u{0}b",
            "/v0/ext/acme-git/.",
        ] {
            assert!(check(&scope, bad).is_err(), "{bad}");
        }
    }

    #[test]
    fn a_query_cannot_smuggle_another_route_in() {
        assert!(check(&ext("a"), "/v0/projects?/v0/ext/a/x").is_err());
        assert!(check(&ext("a"), "/v0/ext/a/x?/../../projects").is_ok());
    }

    #[test]
    fn the_extension_header_names_the_origin_and_replaces_anything_a_pane_sent() {
        let name = HeaderName::from_static(EXTENSION_HEADER);
        let mut forged = HeaderMap::new();
        forged.insert(name.clone(), HeaderValue::from_static("victim"));
        stamp(&mut forged, &ext("acme"));
        assert_eq!(forged.get(&name).unwrap(), "acme", "extension origin");
        assert_eq!(forged.get_all(&name).iter().count(), 1, "one value");
        stamp(&mut forged, &PaneOrigin::Core);
        assert!(forged.get(&name).is_none(), "core carries none");
    }
}
