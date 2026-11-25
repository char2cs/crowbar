use webview::{ProtocolRequest, ProtocolResponse};

const NO_CONTENT: u16 = 204;
const PREFLIGHT_METHOD: &str = "OPTIONS";
const ALLOWED_METHODS: &str = "GET, POST, PUT, PATCH, DELETE";
const DEFAULT_HEADERS: &str = "content-type";
const PREFLIGHT_MAX_AGE: &str = "600";
const GRANTS: [&str; 3] = [
    "access-control-allow-origin",
    "access-control-allow-credentials",
    "access-control-expose-headers",
];

pub(crate) fn page_origin(request: &ProtocolRequest) -> Option<String> {
    header(&request.headers, "origin").map(str::to_owned)
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
    let asked = header(headers, "access-control-request-headers").unwrap_or(DEFAULT_HEADERS);
    Some(ProtocolResponse {
        status: NO_CONTENT,
        headers: vec![
            (
                "access-control-allow-methods".into(),
                ALLOWED_METHODS.into(),
            ),
            ("access-control-allow-headers".into(), asked.into()),
            ("access-control-max-age".into(), PREFLIGHT_MAX_AGE.into()),
        ],
        body: Vec::new(),
    })
}

pub(crate) fn readable_by(
    origin: Option<&str>,
    mut response: ProtocolResponse,
) -> ProtocolResponse {
    let Some(origin) = origin else {
        return response;
    };
    response
        .headers
        .retain(|(key, _)| !GRANTS.iter().any(|grant| key.eq_ignore_ascii_case(grant)));
    response.headers.extend([
        ("access-control-allow-origin".into(), origin.to_owned()),
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
    use super::*;

    const PAGE: &str = "crowbar://local";

    fn preflight(
        method: &str,
        headers: &[(&str, &str)],
    ) -> Option<ProtocolResponse> {
        let headers: Vec<(String, String)> = headers
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect();
        asked_preflight(method, &headers)
    }

    fn header_of<'a>(
        response: &'a ProtocolResponse,
        name: &str,
    ) -> Option<&'a str> {
        response
            .headers
            .iter()
            .find(|(key, _)| key.eq_ignore_ascii_case(name))
            .map(|(_, value)| value.as_str())
    }

    #[test]
    fn a_page_reads_what_another_host_answered_it() {
        let answer = ProtocolResponse {
            status: 200,
            headers: vec![("x-crowbar-diff-truncated".into(), "1".into())],
            body: Vec::new(),
        };
        let granted = readable_by(Some(PAGE), answer.clone());
        assert_eq!(
            header_of(&granted, "access-control-allow-origin"),
            Some(PAGE)
        );
        assert_eq!(
            header_of(&granted, "access-control-expose-headers"),
            Some("*")
        );
        assert_eq!(header_of(&granted, "vary"), Some("Origin"));
        let unasked = readable_by(None, answer);
        assert_eq!(header_of(&unasked, "access-control-allow-origin"), None);
    }

    #[test]
    fn a_preflight_is_answered_for_the_methods_and_headers_the_page_asks() {
        let asked = [
            ("Origin", PAGE),
            ("Access-Control-Request-Method", "POST"),
            ("Access-Control-Request-Headers", "content-type"),
        ];
        let answer = preflight("OPTIONS", &asked).map(|answer| readable_by(Some(PAGE), answer));
        let answer = answer.expect("a preflight is answered by the shell");
        assert_eq!(answer.status, 204);
        assert_eq!(
            header_of(&answer, "access-control-allow-origin"),
            Some(PAGE)
        );
        assert_eq!(
            header_of(&answer, "access-control-allow-headers"),
            Some("content-type")
        );
        assert!(
            header_of(&answer, "access-control-allow-methods").is_some_and(|m| m.contains("POST"))
        );
        assert!(
            preflight("OPTIONS", &[("Origin", PAGE)]).is_none(),
            "a plain OPTIONS"
        );
        assert!(preflight("GET", &asked).is_none(), "a GET");
    }
}
