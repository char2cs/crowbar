use tauri::http;

use super::PROXY_HEADER;

#[derive(Clone)]
pub(crate) struct ProxyAnswer(());

pub(crate) fn error_response(
    status: http::StatusCode,
    msg: &str,
) -> http::Response<Vec<u8>> {
    let mut resp = http::Response::new(msg.as_bytes().to_vec());
    *resp.status_mut() = status;
    resp.headers_mut().insert(
        http::header::CONTENT_TYPE,
        http::HeaderValue::from_static("text/plain"),
    );
    resp.extensions_mut().insert(ProxyAnswer(()));
    resp
}

pub(crate) fn proxy_error(
    status: http::StatusCode,
    marker: &'static str,
    msg: &str,
) -> http::Response<Vec<u8>> {
    let mut resp = error_response(status, msg);
    resp.headers_mut()
        .insert(PROXY_HEADER, http::HeaderValue::from_static(marker));
    resp
}

// PLATFORM: webkit hides any answer without CORS headers (status and marker too) because the
// PLATFORM: webkit needs the proxy to grant its own answers, while a daemon grants its own.
pub(crate) fn readable_by(
    origin: Option<http::HeaderValue>,
    mut resp: http::Response<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let Some(origin) = origin else {
        return resp;
    };
    if resp.extensions().get::<ProxyAnswer>().is_none() {
        return resp;
    }
    let headers = resp.headers_mut();
    headers.insert(http::header::ACCESS_CONTROL_ALLOW_ORIGIN, origin);
    headers.insert(
        http::header::ACCESS_CONTROL_EXPOSE_HEADERS,
        http::HeaderValue::from_static(PROXY_HEADER),
    );
    headers.append(http::header::VARY, http::HeaderValue::from_static("Origin"));
    resp
}

#[cfg(test)]
mod tests {
    use super::*;

    const PAGE: &str = "tauri://localhost";

    fn allow_origin(resp: &http::Response<Vec<u8>>) -> Option<&str> {
        let value = resp
            .headers()
            .get(http::header::ACCESS_CONTROL_ALLOW_ORIGIN);
        value.map(|v| v.to_str().unwrap())
    }

    #[test]
    fn the_proxy_grants_the_page_only_its_own_answers() {
        type Case = (bool, bool, Option<&'static str>);
        const CASES: &[Case] = &[
            (true, true, Some(PAGE)),
            (true, false, None),
            (false, true, None),
            (false, false, None),
        ];
        for (from_proxy, with_origin, expected) in CASES {
            let resp = match from_proxy {
                true => error_response(http::StatusCode::SERVICE_UNAVAILABLE, "down"),
                false => http::Response::new(Vec::new()),
            };
            let origin = with_origin.then(|| http::HeaderValue::from_static(PAGE));
            let resp = readable_by(origin, resp);
            assert_eq!(
                allow_origin(&resp),
                *expected,
                "proxy={from_proxy} origin={with_origin}"
            );
        }
    }

    #[test]
    fn a_readable_proxy_answer_exposes_its_marker_and_varies_by_origin() {
        let resp = proxy_error(
            http::StatusCode::SERVICE_UNAVAILABLE,
            "host-unreachable",
            "down",
        );
        let origin = Some(http::HeaderValue::from_static(PAGE));

        let resp = readable_by(origin, resp);

        let header = |name| resp.headers().get(name).map(|v| v.to_str().unwrap());
        assert_eq!(resp.status(), 503);
        assert_eq!(header(PROXY_HEADER), Some("host-unreachable"));
        assert_eq!(header("access-control-expose-headers"), Some(PROXY_HEADER));
        assert_eq!(header("vary"), Some("Origin"));
        assert_eq!(resp.body().as_slice(), b"down");
    }
}
