use http::{HeaderName, HeaderValue, Method, Request, Response};
use protocol::{DaemonRequest, DaemonResponse, HostId};

use crate::access_scope::AccessScope;
use crate::error::Error;
use crate::response::error_response;

const LOCAL_AUTHORITY: &str = "localhost";

fn authority(host: &HostId) -> &str {
    match host.as_str() {
        "local" => LOCAL_AUTHORITY,
        other => other,
    }
}

pub(crate) type Routed = (AccessScope, Request<Vec<u8>>);

pub(crate) fn to_http(request: &DaemonRequest) -> Result<Routed, Error> {
    let scope = AccessScope::of(request.origin, request.extension.as_ref())?;
    let uri = format!(
        "crowbar://{}{}",
        authority(&request.host),
        request.path_and_query
    );
    let method = Method::from_bytes(request.method.as_bytes())
        .map_err(|_| Error::BadGrant("the request method is not valid"))?;
    let mut http_request = Request::builder()
        .method(method)
        .uri(uri)
        .body(request.body.clone())?;
    for (name, value) in &request.headers {
        let name = HeaderName::from_bytes(name.as_bytes())
            .map_err(|_| Error::BadGrant("a request header name is not valid"))?;
        let value = HeaderValue::from_str(value)
            .map_err(|_| Error::BadGrant("a request header value is not valid"))?;
        http_request.headers_mut().append(name, value);
    }
    Ok((scope, http_request))
}

pub(crate) fn refusal(error: &Error) -> DaemonResponse {
    let status = if matches!(error, Error::BadGrant(_)) {
        403
    } else {
        400
    };
    from_http(&error_response(status, &error.to_string()))
}

pub(crate) fn from_http(response: &Response<Vec<u8>>) -> DaemonResponse {
    let headers = response
        .headers()
        .iter()
        .filter_map(|(name, value)| Some((name.to_string(), value.to_str().ok()?.to_owned())))
        .collect();
    DaemonResponse {
        status: response.status().as_u16(),
        headers,
        body: response.body().clone(),
    }
}

#[cfg(test)]
#[path = "daemon_wire_tests.rs"]
mod tests;
