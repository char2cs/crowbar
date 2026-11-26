use protocol::PaneOrigin;
use webview::{ProtocolRequest, ProtocolResponse};

use crate::scheme::access::page_caller::PageCaller;
use crate::scheme::route::Route;

pub(crate) fn allows(
    origin: &PaneOrigin,
    route: &Route,
) -> bool {
    match (origin, route) {
        (_, Route::Relay { .. })
        | (PaneOrigin::Core, Route::CoreBundle { .. } | Route::Daemon { .. } | Route::Menu) => true,
        (PaneOrigin::Extension(own), Route::ExtensionBundle { extension, .. }) => own == extension,
        (PaneOrigin::Extension(own), Route::Daemon { path_and_query, .. }) => {
            is_plain(path_and_query) && path_and_query.starts_with(&format!("/v0/ext/{own}/"))
        },
        (PaneOrigin::Core, Route::ExtensionBundle { .. })
        | (PaneOrigin::Extension(_), Route::CoreBundle { .. } | Route::Menu) => false,
    }
}

const TRAVERSAL_MARKERS: [&str; 5] = ["..", "%2e", "%2f", "%5c", "\\"];

fn is_plain(path_and_query: &str) -> bool {
    let lowered = path_and_query.to_ascii_lowercase();
    TRAVERSAL_MARKERS
        .iter()
        .all(|marker| !lowered.contains(marker))
}

const NO_CONTENT: u16 = 204;
const PREFLIGHT_METHOD: &str = "OPTIONS";
const ALLOWED_METHODS: &str = "GET, POST, PUT, PATCH, DELETE";
const PANE_HEADERS: &str = "content-type, idempotency-key, crowbar-discard-work";
const PAGE_ORIGIN: &str = "crowbar://local";
const PREFLIGHT_MAX_AGE: &str = "600";
const GRANTS: [&str; 3] = [
    "access-control-allow-origin",
    "access-control-allow-credentials",
    "access-control-expose-headers",
];

pub(crate) fn caller_of(request: &ProtocolRequest) -> PageCaller {
    match header(&request.headers, "origin") {
        None => PageCaller::Unstated,
        Some(PAGE_ORIGIN) => PageCaller::ShellPage,
        Some(_) => PageCaller::Stranger,
    }
}

pub(crate) fn preflight(request: &ProtocolRequest) -> Option<ProtocolResponse> {
    asked_preflight(&request.method, &request.headers)
}

fn asked_preflight(
    method: &str,
    headers: &[(String, String)],
) -> Option<ProtocolResponse> {
    if !method.eq_ignore_ascii_case(PREFLIGHT_METHOD) {
        return None;
    }
    header(headers, "access-control-request-method")?;
    Some(ProtocolResponse {
        status: NO_CONTENT,
        headers: vec![
            (
                "access-control-allow-methods".into(),
                ALLOWED_METHODS.into(),
            ),
            ("access-control-allow-headers".into(), PANE_HEADERS.into()),
            ("access-control-max-age".into(), PREFLIGHT_MAX_AGE.into()),
        ],
        body: Vec::new(),
    })
}

pub(crate) fn readable_by(
    caller: PageCaller,
    mut response: ProtocolResponse,
) -> ProtocolResponse {
    if caller != PageCaller::ShellPage {
        return response;
    }
    response
        .headers
        .retain(|(key, _)| !GRANTS.iter().any(|grant| key.eq_ignore_ascii_case(grant)));
    response.headers.extend([
        ("access-control-allow-origin".into(), PAGE_ORIGIN.into()),
        ("access-control-expose-headers".into(), "*".into()),
        ("vary".into(), "Origin".into()),
    ]);
    response
}

fn header<'a>(
    headers: &'a [(String, String)],
    name: &str,
) -> Option<&'a str> {
    headers
        .iter()
        .find(|(key, _)| key.eq_ignore_ascii_case(name))
        .map(|(_, value)| value.as_str())
}

#[cfg(test)]
mod tests {
    use protocol::{ExtensionId, HostId};

    use super::*;

    type Case = (&'static str, PaneOrigin, Route, bool);

    fn git() -> PaneOrigin {
        PaneOrigin::Extension(ExtensionId::new("git").unwrap())
    }

    fn daemon(path: &str) -> Route {
        let host = HostId::new("local").unwrap();
        let path_and_query = path.into();
        Route::Daemon {
            host,
            path_and_query,
        }
    }

    fn bundle(name: &str) -> Route {
        let extension = ExtensionId::new(name).unwrap();
        let path = "a".into();
        Route::ExtensionBundle { extension, path }
    }

    fn core_bundle() -> Route {
        let path = String::new();
        Route::CoreBundle { path }
    }

    fn relay() -> Route {
        let host = HostId::new("local").unwrap();
        Route::Relay { host }
    }

    fn core() -> PaneOrigin {
        PaneOrigin::Core
    }

    fn cases() -> Vec<Case> {
        vec![
            ("core: the core bundle", core(), core_bundle(), true),
            ("core: the daemon", core(), daemon("/v0/threads"), true),
            ("core: no ext bundle", core(), bundle("git"), false),
            ("core: its relay", core(), relay(), true),
            ("ext: its relay", git(), relay(), true),
            ("ext: its own bundle", git(), bundle("git"), true),
            ("ext: no foreign bundle", git(), bundle("draw"), false),
            ("ext: no core bundle", git(), core_bundle(), false),
            ("ext: its routes", git(), daemon("/v0/ext/git/a"), true),
            ("lookalike", git(), daemon("/v0/ext/gitx/x"), false),
            ("foreign", git(), daemon("/v0/ext/draw/x"), false),
            ("general api", git(), daemon("/v0/threads"), false),
            ("dots", git(), daemon("/v0/ext/git/../x"), false),
            ("%2e", git(), daemon("/v0/ext/git/%2e%2e/x"), false),
            ("%2E", git(), daemon("/v0/ext/git/%2E%2E/x"), false),
            ("%2f", git(), daemon("/v0/ext/git/a%2fb"), false),
        ]
    }

    #[test]
    fn every_origin_reaches_exactly_what_it_owns() {
        for (name, origin, route, expected) in cases() {
            assert_eq!(allows(&origin, &route), expected, "{name}");
        }
    }
}
