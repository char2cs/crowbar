use http::{HeaderValue, StatusCode, header};
use tokio::sync::oneshot;
use tokio::time::Instant;
use tokio_tungstenite::tungstenite::handshake::server::{
    Callback, ErrorResponse, Request, Response,
};

use crate::access_scope::AccessScope;
use crate::admission::Admission;
use crate::relay_protocol::{RELAY_SUBPROTOCOL, TOKEN_SUBPROTOCOL_PREFIX};
use crate::relay_state::RelayState;
use std::sync::{Arc, PoisonError};

pub(crate) struct Gate {
    state: Arc<RelayState>,
    admitted: oneshot::Sender<Admission>,
}

impl Gate {
    pub(crate) fn new(state: Arc<RelayState>, admitted: oneshot::Sender<Admission>) -> Self {
        Self { state, admitted }
    }
}

impl Callback for Gate {
    fn on_request(
        self,
        request: &Request,
        mut response: Response,
    ) -> Result<Response, ErrorResponse> {
        let admission = inspect(&self.state, request).map_err(refusal)?;
        self.admitted
            .send(admission)
            .map_err(|_| refusal(StatusCode::SERVICE_UNAVAILABLE))?;
        response.headers_mut().insert(
            header::SEC_WEBSOCKET_PROTOCOL,
            HeaderValue::from_static(RELAY_SUBPROTOCOL),
        );
        Ok(response)
    }
}

fn refusal(status: StatusCode) -> ErrorResponse {
    let mut response = ErrorResponse::new(None);
    *response.status_mut() = status;
    response
}

fn inspect(state: &RelayState, request: &Request) -> Result<Admission, StatusCode> {
    check_host(request, state.port)?;
    let origin = single_origin(request)?;
    let offered = offered_protocols(request);
    if !offered.iter().any(|p| p == RELAY_SUBPROTOCOL) {
        return Err(StatusCode::BAD_REQUEST);
    }
    let token = offered
        .iter()
        .find_map(|p| p.strip_prefix(TOKEN_SUBPROTOCOL_PREFIX))
        .ok_or(StatusCode::UNAUTHORIZED)?;
    let scope = state
        .tokens
        .lock()
        .unwrap_or_else(PoisonError::into_inner)
        .authorize(token, Instant::now())
        .ok_or(StatusCode::UNAUTHORIZED)?;
    if !state.policy.allows(&scope, origin) {
        return Err(StatusCode::FORBIDDEN);
    }
    let path_and_query = request.uri().path_and_query().map_or("/", |pq| pq.as_str());
    let access =
        AccessScope::of(scope.kind, scope.extension.as_ref()).map_err(|_| StatusCode::FORBIDDEN)?;
    access
        .check(path_and_query)
        .map_err(|_| StatusCode::FORBIDDEN)?;
    Ok(Admission {
        scope,
        path_and_query: path_and_query.to_owned(),
    })
}

fn check_host(request: &Request, port: u16) -> Result<(), StatusCode> {
    let expected = format!("127.0.0.1:{port}");
    let host = request
        .headers()
        .get(header::HOST)
        .and_then(|v| v.to_str().ok());
    if host == Some(expected.as_str()) {
        return Ok(());
    }
    Err(StatusCode::FORBIDDEN)
}

fn single_origin(request: &Request) -> Result<&str, StatusCode> {
    let mut origins = request.headers().get_all(header::ORIGIN).iter();
    match (origins.next(), origins.next()) {
        (Some(origin), None) => origin.to_str().map_err(|_| StatusCode::FORBIDDEN),
        _ => Err(StatusCode::FORBIDDEN),
    }
}

fn offered_protocols(request: &Request) -> Vec<String> {
    request
        .headers()
        .get_all(header::SEC_WEBSOCKET_PROTOCOL)
        .iter()
        .filter_map(|value| value.to_str().ok())
        .flat_map(|list| list.split(','))
        .map(|protocol| protocol.trim().to_owned())
        .collect()
}

#[cfg(test)]
#[path = "gate_tests.rs"]
mod tests;
