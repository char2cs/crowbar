use std::borrow::Cow;
use std::sync::Arc;

use wry::dpi::{LogicalPosition, LogicalSize};
use wry::http::Response;
use wry::{Rect, WebViewBuilder};

use crate::bounds::Bounds;
use crate::protocol_handler::ProtocolHandler;
use crate::protocol_request::ProtocolRequest;
use crate::protocol_response::ProtocolResponse;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;

pub(crate) fn register_scheme<'a>(
    builder: WebViewBuilder<'a>,
    scheme: &str,
    protocol: Arc<dyn ProtocolHandler>,
) -> WebViewBuilder<'a> {
    builder.with_asynchronous_custom_protocol(scheme.to_owned(), move |view, request, responder| {
        let Some(web_view) = WebViewId::parse(view) else {
            responder.respond(plain_response(400));
            return;
        };
        let request = ProtocolRequest {
            web_view,
            uri: request.uri().to_string(),
            method: request.method().to_string(),
            headers: request
                .headers()
                .iter()
                .map(|(name, value)| {
                    (
                        name.to_string(),
                        String::from_utf8_lossy(value.as_bytes()).into_owned(),
                    )
                })
                .collect(),
            body: request.into_body(),
        };
        protocol.handle(
            request,
            Box::new(move |response| responder.respond(into_http(response))),
        );
    })
}

pub(crate) fn common_attributes<'a>(
    builder: WebViewBuilder<'a>,
    id: &'a str,
    config: &WebViewConfig,
) -> WebViewBuilder<'a> {
    let on_ipc = Arc::clone(&config.on_ipc);
    let builder = builder
        .with_id(id)
        .with_url(&config.url)
        .with_bounds(to_rect(config.bounds))
        .with_visible(config.visibility == Visibility::Shown)
        .with_ipc_handler(move |request| on_ipc(request.into_body()));
    match &config.initialization_script {
        Some(script) => builder.with_initialization_script(script),
        None => builder,
    }
}

pub(crate) fn to_rect(bounds: Bounds) -> Rect {
    Rect {
        position: LogicalPosition::new(bounds.x, bounds.y).into(),
        size: LogicalSize::new(bounds.width.max(1.0), bounds.height.max(1.0)).into(),
    }
}

fn plain_response(status: u16) -> Response<Cow<'static, [u8]>> {
    Response::builder()
        .status(status)
        .body(Cow::Borrowed(&[][..]))
        .unwrap_or_default()
}

fn into_http(response: ProtocolResponse) -> Response<Cow<'static, [u8]>> {
    let mut builder = Response::builder().status(response.status);
    for (name, value) in response.headers {
        builder = builder.header(name, value);
    }
    builder
        .body(Cow::Owned(response.body))
        .unwrap_or_else(|_| plain_response(500))
}

#[cfg(test)]
#[path = "builder_support_tests.rs"]
mod tests;
