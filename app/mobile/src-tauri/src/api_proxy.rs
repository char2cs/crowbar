use std::sync::Arc;
use std::time::Duration;

use tauri::http;
use tauri::{AppHandle, Manager, Runtime, UriSchemeContext, UriSchemeResponder};

use crate::host_route::is_remote_host;
use crate::host_tokens::TokenStore;
use crate::hosts::HostRegistry;
use proxy_answer::{error_response, readable_by};

mod channel;
pub(crate) mod pairing;
pub(crate) mod probe;
mod proxy_answer;
pub(crate) mod remote;

const PROXY_TIMEOUT: Duration = Duration::from_secs(300);

pub(crate) const PROXY_HEADER: &str = "x-crowbar-proxy";
pub(crate) const HOST_UNREACHABLE: &str = "host-unreachable";
pub(crate) const HOST_IDENTITY_CHANGED: &str = "host-identity-changed";

fn remote_host_id(uri: &http::Uri) -> Option<&str> {
    uri.host().filter(|host| is_remote_host(host))
}

#[expect(
    clippy::needless_pass_by_value,
    reason = "tauri hands the scheme context to the handler by value"
)]
pub(crate) fn handle_request<R: Runtime>(
    ctx: UriSchemeContext<'_, R>,
    request: http::Request<Vec<u8>>,
    responder: UriSchemeResponder,
) {
    let app = ctx.app_handle().clone();
    let remote_host = remote_host_id(request.uri()).map(str::to_owned);
    let origin = request.headers().get(http::header::ORIGIN).cloned();
    tauri::async_runtime::spawn(async move {
        let routed = route(&app, remote_host, request);
        let resp = match tokio::time::timeout(PROXY_TIMEOUT, routed).await {
            Ok(resp) => resp,
            Err(_) => error_response(
                http::StatusCode::GATEWAY_TIMEOUT,
                "crowbar daemon did not answer in time",
            ),
        };
        respond_on_main(&app, responder, readable_by(origin, resp));
    });
}

async fn route<R: Runtime>(
    app: &AppHandle<R>,
    remote_host: Option<String>,
    request: http::Request<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let Some(host_id) = remote_host else {
        return error_response(
            http::StatusCode::SERVICE_UNAVAILABLE,
            "no local daemon on this device",
        );
    };
    remote::forward_remote(
        &app.state::<HostRegistry>(),
        &app.state::<Arc<dyn TokenStore>>(),
        &host_id,
        request,
    )
    .await
}

// PLATFORM: WebKit delivers task cancellation on the main thread; answering a stopped task from
// PLATFORM: webkit answering a stopped task from a worker races it, and its NSException there aborts the app.
fn respond_on_main<R: Runtime>(
    app: &AppHandle<R>,
    responder: UriSchemeResponder,
    resp: http::Response<Vec<u8>>,
) {
    if let Err(error) = app.run_on_main_thread(move || responder.respond(resp)) {
        log::warn!("api proxy: could not answer on the main thread: {error}");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_non_localhost_authority_routes_to_a_remote_host() {
        const CASES: &[(&str, Option<&str>)] = &[
            ("crowbar://localhost/v0/projects", None),
            ("crowbar://localhost:80/v0/projects", None),
            ("crowbar://host-1/v0/projects", Some("host-1")),
        ];
        for (uri, expected) in CASES {
            let uri: http::Uri = uri.parse().unwrap();
            assert_eq!(remote_host_id(&uri), *expected, "{uri}");
        }
    }
}
