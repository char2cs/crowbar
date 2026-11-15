use http::{HeaderMap, HeaderName, HeaderValue};
use protocol::PaneOrigin;

use crate::error::Error;

const API_PREFIX: &str = "/v0";
const EXTENSION_ROUTES: &str = "/v0/ext/";
const ENCODED_ESCAPES: [&str; 4] = ["%2e", "%2f", "%5c", "%00"];

/// Header the transport sets on every extension-scoped request, HTTP and WebSocket alike, and
/// strips from every other, so a daemon can apply its own per-origin check without trusting the
/// page. Its value is the extension id.
pub const EXTENSION_HEADER: &str = "x-crowbar-extension";

pub(crate) fn ensure_path_allowed(
    origin: &PaneOrigin,
    path_and_query: &str,
) -> Result<(), Error> {
    let path = path_and_query.split(['?', '#']).next().unwrap_or_default();
    if is_plain_path(path) && allows(origin, path) {
        return Ok(());
    }
    Err(Error::Forbidden(path_and_query.to_owned()))
}

pub(crate) fn stamp_extension_header(
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
    let is_malformed = !path.starts_with('/') || path.contains("//");
    if is_malformed || path.bytes().any(control) {
        return false;
    }
    let lowered = path.to_ascii_lowercase();
    let has_encoded_escape = ENCODED_ESCAPES.iter().any(|bad| lowered.contains(bad));
    if has_encoded_escape {
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

    #[derive(Clone, Copy)]
    enum Who {
        Core,
        Extension(&'static str),
    }

    /// (who asks, the request target, whether the scope lets it through)
    const CASES: [(Who, &str, bool); 29] = [
        (Who::Core, "/v0", true),
        (Who::Core, "/v0/projects?x=1", true),
        (Who::Core, "/v0/ext/git/call", true),
        (Who::Core, "/", false),
        (Who::Core, "/debug/pprof/goroutine", false),
        (Who::Core, "/v00", false),
        (Who::Core, "v0/x", false),
        (Who::Core, "/v0/../debug", false),
        (Who::Core, "/v0/%2e%2e/x", false),
        (Who::Extension("acme-git"), "/v0/ext/acme-git", true),
        (Who::Extension("acme-git"), "/v0/ext/acme-git/call", true),
        (
            Who::Extension("acme-git"),
            "/v0/ext/acme-git/blobs/abc?x=1",
            true,
        ),
        (Who::Extension("acme-git"), "/v0/projects", false),
        (Who::Extension("acme-git"), "/v0/ext/acme-gitx/call", false),
        (Who::Extension("acme-git"), "/v0/ext/acme/call", false),
        (Who::Extension("acme-git"), "/v0/ext/other/blobs/abc", false),
        (Who::Extension("acme-git"), "/v0/ext/", false),
        (Who::Extension("acme-git"), "/v0/ext//acme-git/call", false),
        (
            Who::Extension("acme-git"),
            "/v0/ext/acme-git/../other/call",
            false,
        ),
        (
            Who::Extension("acme-git"),
            "/v0/ext/acme-git/%2E%2E/other",
            false,
        ),
        (
            Who::Extension("acme-git"),
            "/v0/ext/acme-git/..%2fother",
            false,
        ),
        (
            Who::Extension("acme-git"),
            "/v0/ext/acme-git\\..\\other",
            false,
        ),
        (
            Who::Extension("acme-git"),
            "/v0/ext/acme-git/a\u{0}b",
            false,
        ),
        (Who::Extension("acme-git"), "/v0/ext/acme-git/.", false),
        (Who::Extension("a"), "/v0/projects?/v0/ext/a/x", false),
        (Who::Extension("a"), "/v0/ext/a/x?/../../projects", true),
        (Who::Extension("a"), "/v0/ext/a", true),
        (Who::Extension("a"), "/v0/ext/b", false),
        (Who::Extension("a"), "/v0", false),
    ];

    #[test]
    fn each_origin_reaches_only_the_routes_its_scope_allows() {
        for (who, target, allowed) in CASES {
            let origin = match who {
                Who::Core => PaneOrigin::Core,
                Who::Extension(id) => ext(id),
            };
            assert_eq!(
                ensure_path_allowed(&origin, target).is_ok(),
                allowed,
                "{target}"
            );
        }
    }

    #[test]
    fn the_extension_header_names_the_origin_and_replaces_anything_a_pane_sent() {
        let name = HeaderName::from_static(EXTENSION_HEADER);
        let mut forged = HeaderMap::new();
        forged.insert(name.clone(), HeaderValue::from_static("victim"));
        stamp_extension_header(&mut forged, &ext("acme"));
        assert_eq!(forged.get(&name).unwrap(), "acme", "extension origin");
        assert_eq!(forged.get_all(&name).iter().count(), 1, "one value");
        stamp_extension_header(&mut forged, &PaneOrigin::Core);
        assert!(forged.get(&name).is_none(), "core carries none");
    }
}
