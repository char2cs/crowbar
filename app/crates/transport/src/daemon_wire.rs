use http::uri::PathAndQuery;
use http::{HeaderName, HeaderValue, Method, Request, Response, Uri};
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

fn routed_uri(authority: &str, target: &str) -> Result<Uri, Error> {
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

pub(crate) fn to_http(request: DaemonRequest) -> Result<Routed, Error> {
    let origin = request.origin().clone();
    let uri = routed_uri(authority(request.host()), request.target().as_str())?;
    let method = Method::from_bytes(request.method().as_str().as_bytes())
        .map_err(|_| Error::InvalidRequest("the request method is not valid"))?;
    let mut builder = Request::builder().method(method).uri(uri);
    if let Some(headers) = builder.headers_mut() {
        for (name, value) in request.headers() {
            let name = HeaderName::from_bytes(name.as_bytes())
                .map_err(|_| Error::InvalidRequest("a request header name is not valid"))?;
            let value = HeaderValue::from_str(value)
                .map_err(|_| Error::InvalidRequest("a request header value is not valid"))?;
            headers.append(name, value);
        }
    }
    let http_request = builder
        .body(request.into_body())
        .map_err(Error::bad_request)?;
    Ok((origin, http_request))
}

pub(crate) fn refusal(error: &Error) -> DaemonResponse {
    let status = if matches!(error, Error::Forbidden(_)) {
        403
    } else {
        400
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
#[path = "daemon_wire_tests.rs"]
mod tests;
