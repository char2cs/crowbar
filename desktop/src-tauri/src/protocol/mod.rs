use bytes::Bytes;
use http_body_util::{BodyExt, Full};
use hyper::Request;
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
use hyperlocal::{UnixConnector, Uri as UnixUri};
use tauri::http::{Response, StatusCode};

use crate::sidecar::socket_path;

pub(crate) fn rewrite_path(uri: &str) -> String {
    if uri.starts_with("crowbar://events") {
        return "/api/v0/events".to_string();
    }
    uri.strip_prefix("crowbar://api")
        .unwrap_or("/")
        .to_string()
}

pub async fn handle(request: tauri::http::Request<Vec<u8>>) -> Response<Vec<u8>> {
    let uri_str = request.uri().to_string();
    let method = request.method().as_str().to_uppercase();

    // Respond to CORS preflight without hitting the Go backend.
    if method == "OPTIONS" {
        return Response::builder()
            .status(204)
            .header("Access-Control-Allow-Origin", "*")
            .header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            .header("Access-Control-Allow-Headers", "Content-Type, Authorization")
            .body(vec![])
            .unwrap();
    }

    let path = rewrite_path(&uri_str);
    let body_bytes = Bytes::from(request.body().clone());

    match proxy_request(&method, &path, body_bytes).await {
        Ok(resp) => resp,
        Err(e) => {
            log::error!("protocol handler error: {e}");
            error_response(StatusCode::BAD_GATEWAY, &e.to_string())
        }
    }
}

async fn proxy_request(
    method: &str,
    path: &str,
    body: Bytes,
) -> Result<Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>> {
    let socket = socket_path();
    let url: hyper::Uri = UnixUri::new(&socket, path).into();

    let client: Client<UnixConnector, Full<Bytes>> =
        Client::builder(TokioExecutor::new()).build(UnixConnector);

    let req = Request::builder()
        .method(method)
        .uri(url)
        .body(Full::new(body))?;

    let resp = client.request(req).await?;
    let status = resp.status();

    let mut builder = Response::builder().status(status.as_u16());
    for (k, v) in resp.headers() {
        builder = builder.header(k, v);
    }

    let body_bytes = resp.into_body().collect().await?.to_bytes().to_vec();
    // Add CORS headers so the Vite dev server origin (localhost:5173) can reach
    // the crowbar:// custom protocol in Tauri dev mode.
    let builder = builder
        .header("Access-Control-Allow-Origin", "*")
        .header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        .header("Access-Control-Allow-Headers", "Content-Type, Authorization");
    Ok(builder.body(body_bytes)?)
}

fn error_response(status: StatusCode, msg: &str) -> Response<Vec<u8>> {
    let body = serde_json::json!({"error": msg}).to_string();
    Response::builder()
        .status(status.as_u16())
        .header("Content-Type", "application/json")
        .header("Access-Control-Allow-Origin", "*")
        .body(body.into_bytes())
        .unwrap()
}

#[cfg(test)]
mod tests {
    use super::rewrite_path;

    #[test]
    fn api_path_rewritten_correctly() {
        assert_eq!(rewrite_path("crowbar://api/api/v0/health"), "/api/v0/health");
    }

    #[test]
    fn events_path_rewritten_correctly() {
        assert_eq!(rewrite_path("crowbar://events"), "/api/v0/events");
    }
}
