use http::uri::PathAndQuery;
use http::{HeaderName, HeaderValue, Method, Request, Response, StatusCode, Uri};
use protocol::{DaemonRequest, DaemonResponse, HostId, PaneOrigin};

use crate::error::Error;
use crate::response::error_response;

const LOCAL_AUTHORITY: &str = "localhost";
const SCHEME: &str = "crowbar";

pub(crate) type Routed = (PaneOrigin, Request<Vec<u8>>);

fn authority(host: &HostId) -> &str {
    match host.as_str() {
        "local" => LOCAL_AUTHORITY,
        other => other,
    }
}

fn routed_uri(
    authority: &str,
    target: &str,
) -> Result<Uri, Error> {
    if !target.starts_with('/') || target.starts_with("//") {
        return Err(Error::InvalidRequest("the request target is not absolute"));
    }
    let path_and_query = PathAndQuery::try_from(target)
        .map_err(|_| Error::InvalidRequest("the request target is not valid"))?;
    let uri = Uri::builder()
        .scheme(SCHEME)
        .authority(authority)
        .path_and_query(path_and_query)
        .build()
        .map_err(Error::bad_request)?;
    if uri.host() != Some(authority) {
        return Err(Error::InvalidRequest("the request target changes the host"));
    }
    Ok(uri)
}

fn header_name(name: &str) -> Result<HeaderName, Error> {
    HeaderName::from_bytes(name.as_bytes())
        .map_err(|_| Error::InvalidRequest("a request header name is not valid"))
}

fn header_value(value: &str) -> Result<HeaderValue, Error> {
    HeaderValue::from_str(value)
        .map_err(|_| Error::InvalidRequest("a request header value is not valid"))
}

pub(crate) fn to_http(request: DaemonRequest) -> Result<Routed, Error> {
    let origin = request.origin().clone();
    let uri = routed_uri(authority(request.host()), request.target().as_str())?;
    let method = Method::from_bytes(request.method().as_str().as_bytes())
        .map_err(|_| Error::InvalidRequest("the request method is not valid"))?;
    let mut builder = Request::builder().method(method).uri(uri);
    for (name, value) in request.headers() {
        builder = builder.header(header_name(name)?, header_value(value)?);
    }
    let http_request = builder
        .body(request.into_body())
        .map_err(Error::bad_request)?;
    Ok((origin, http_request))
}

pub(crate) fn refusal(error: &Error) -> DaemonResponse {
    let status = match error {
        Error::Forbidden(_) => StatusCode::FORBIDDEN,
        _ => StatusCode::BAD_REQUEST,
    };
    from_http(error_response(status, &error.to_string()))
}

pub(crate) fn from_http(response: Response<Vec<u8>>) -> DaemonResponse {
    let (parts, body) = response.into_parts();
    let headers = parts
        .headers
        .iter()
        .map(|(name, value)| {
            let text = String::from_utf8_lossy(value.as_bytes()).into_owned();
            (name.to_string(), text)
        })
        .collect();
    DaemonResponse::new(parts.status.as_u16(), headers, body)
}

#[cfg(test)]
mod tests {
    use protocol::{ExtensionId, HttpMethod, RequestTarget};

    use super::*;

    fn request(
        host: &str,
        path: &str,
    ) -> DaemonRequest {
        DaemonRequest::new(
            HostId::new(host).unwrap(),
            PaneOrigin::Core,
            HttpMethod::new("POST").unwrap(),
            RequestTarget::new(path).unwrap(),
        )
        .with_headers(vec![("content-type".into(), "application/json".into())])
        .unwrap()
        .with_body(b"{}".to_vec())
    }

    #[test]
    fn the_local_host_is_spelled_localhost_on_the_wire() {
        let (origin, http_request) = to_http(request("local", "/v0/x?y=1")).unwrap();
        assert_eq!(origin, PaneOrigin::Core);
        assert_eq!(
            http_request.uri().to_string(),
            "crowbar://localhost/v0/x?y=1"
        );
        assert_eq!(http_request.method(), Method::POST);
        assert_eq!(http_request.headers()["content-type"], "application/json");
        assert_eq!(http_request.body(), b"{}");
        let (_, remote) = to_http(request("host-1", "/v0/x")).unwrap();
        assert_eq!(remote.uri().host(), Some("host-1"));
    }

    #[test]
    fn the_extension_origin_travels_with_the_request() {
        let acme = PaneOrigin::Extension(ExtensionId::new("acme").unwrap());
        let built = DaemonRequest::new(
            HostId::new("local").unwrap(),
            acme.clone(),
            HttpMethod::new("GET").unwrap(),
            RequestTarget::new("/v0/ext/acme/x").unwrap(),
        );
        assert_eq!(to_http(built).unwrap().0, acme);
    }

    #[test]
    fn a_target_can_never_change_the_host_the_request_is_dialled_to() {
        const CASES: [&str; 8] = [
            "@host-2/v0/x",
            "-x/v0/x",
            ":80/v0/x",
            ".evil/v0/x",
            "\\host-2/v0/x",
            "//host-2/v0/x",
            "v0/x",
            "",
        ];
        for target in CASES {
            assert!(routed_uri("localhost", target).is_err(), "{target:?}");
        }
        let uri = routed_uri("localhost", "/v0/x@host-2/y").unwrap();
        assert_eq!(
            uri.host(),
            Some("localhost"),
            "an @ after the first slash is path text"
        );
    }

    #[test]
    fn a_header_that_cannot_be_spelled_is_refused_not_guessed() {
        let bad_name = request("local", "/v0/x")
            .with_headers(vec![("bad name".into(), "v".into())])
            .unwrap();
        let bad_value = request("local", "/v0/x")
            .with_headers(vec![("x-a".into(), "line\nbreak".into())])
            .unwrap();
        let cases = [bad_name, bad_value];
        for case in cases {
            assert!(matches!(to_http(case), Err(Error::InvalidRequest(_))));
        }
    }

    #[test]
    fn a_response_keeps_status_headers_and_body_and_non_ascii_values() {
        let mut answer = error_response(StatusCode::SERVICE_UNAVAILABLE, "down");
        answer
            .headers_mut()
            .insert("x-extra", HeaderValue::from_static("1"));
        answer.headers_mut().insert(
            "content-disposition",
            HeaderValue::from_bytes("attachment; filename=\u{fc}.txt".as_bytes()).unwrap(),
        );
        let wire = from_http(answer);
        assert_eq!(wire.status(), 503);
        assert_eq!(wire.body(), b"down");
        assert!(
            wire.headers()
                .contains(&("x-extra".to_owned(), "1".to_owned()))
        );
        assert!(
            wire.headers().contains(&(
                "content-disposition".to_owned(),
                "attachment; filename=\u{fc}.txt".to_owned()
            )),
            "a UTF-8 header value is kept whole, not dropped"
        );
    }

    #[test]
    fn a_refusal_is_403_for_policy_and_400_for_a_malformed_request() {
        assert_eq!(refusal(&Error::Forbidden("/x".into())).status(), 403);
        assert_eq!(refusal(&Error::InvalidRequest("x")).status(), 400);
    }
}
