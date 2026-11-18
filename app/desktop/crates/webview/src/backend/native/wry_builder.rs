use std::borrow::Cow;
use std::sync::Arc;

use wry::dpi::{LogicalPosition, LogicalSize};
use wry::http::header::{HeaderName, HeaderValue};
use wry::http::{Request, Response};
use wry::{DragDropEvent, PageLoadEvent, Rect, RequestAsyncResponder, WebViewBuilder};

use crate::scheme::ProtocolHandler;
use crate::scheme::ProtocolRequest;
use crate::scheme::ProtocolResponse;
use crate::view::Bounds;
use crate::view::Visibility;
use crate::view::WebViewConfig;
use crate::view::WebViewId;
use crate::view::{PageEvent, PageHandler};

const HTTP_BAD_REQUEST: u16 = 400;
const HTTP_INTERNAL_ERROR: u16 = 500;

fn header_pair((name, value): (&HeaderName, &HeaderValue)) -> (String, String) {
    (
        name.to_string(),
        String::from_utf8_lossy(value.as_bytes()).into_owned(),
    )
}

fn protocol_request(
    web_view: WebViewId,
    request: Request<Vec<u8>>,
) -> ProtocolRequest {
    ProtocolRequest {
        web_view,
        uri: request.uri().to_string(),
        method: request.method().to_string(),
        headers: request.headers().iter().map(header_pair).collect(),
        body: request.into_body(),
    }
}

pub(crate) fn register_scheme<'a>(
    builder: WebViewBuilder<'a>,
    scheme: &str,
    protocol: Arc<dyn ProtocolHandler>,
) -> WebViewBuilder<'a> {
    builder.with_asynchronous_custom_protocol(scheme.to_owned(), move |view, request, responder| {
        serve(&*protocol, (view, request), responder);
    })
}

fn serve(
    protocol: &dyn ProtocolHandler,
    (view, request): (&str, Request<Vec<u8>>),
    responder: RequestAsyncResponder,
) {
    let Some(web_view) = WebViewId::parse(view) else {
        responder.respond(plain_response(HTTP_BAD_REQUEST));
        return;
    };
    protocol.handle(
        protocol_request(web_view, request),
        Box::new(move |response| responder.respond(into_http(response))),
    );
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
    let builder = match config.background {
        Some(color) => builder.with_background_color(color.rgba()),
        None => builder,
    };
    let builder = match &config.on_page {
        Some(handler) => watch_pages(builder, handler),
        None => builder,
    };
    match &config.initialization_script {
        Some(script) => builder.with_initialization_script(script),
        None => builder,
    }
}

fn watch_pages<'a>(
    builder: WebViewBuilder<'a>,
    handler: &PageHandler,
) -> WebViewBuilder<'a> {
    let on_navigate = Arc::clone(handler);
    let on_load = Arc::clone(handler);
    let on_title = Arc::clone(handler);
    let on_drop = Arc::clone(handler);
    builder
        .with_navigation_handler(move |url| on_navigate(PageEvent::Navigating(url)))
        .with_on_page_load_handler(move |event, url| {
            let event = match event {
                PageLoadEvent::Started => PageEvent::Loading(url),
                PageLoadEvent::Finished => PageEvent::Loaded(url),
            };
            on_load(event);
        })
        .with_document_title_changed_handler(move |title| {
            on_title(PageEvent::Title(title));
        })
        .with_drag_drop_handler(move |event| {
            forward_drop(&on_drop, event);
            false
        })
}

fn forward_drop(
    handler: &PageHandler,
    event: DragDropEvent,
) {
    if let Some(dropped) = files_dropped(event) {
        handler(dropped);
    }
}

fn files_dropped(event: DragDropEvent) -> Option<PageEvent> {
    match event {
        DragDropEvent::Drop { paths, .. } if !paths.is_empty() => {
            Some(PageEvent::FilesDropped(paths))
        },
        _ => None,
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
    let builder = response.headers.into_iter().fold(
        Response::builder().status(response.status),
        |builder, (name, value)| builder.header(name, value),
    );
    builder
        .body(Cow::Owned(response.body))
        .unwrap_or_else(|_| plain_response(HTTP_INTERNAL_ERROR))
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static str, f64, f64, f64, f64);

    #[test]
    fn only_a_drop_with_files_is_reported() {
        let path = std::path::PathBuf::from("/tmp/a.txt");
        let at = (3, 4);
        let cases = [
            (
                DragDropEvent::Drop {
                    paths: vec![path.clone()],
                    position: at,
                },
                Some(PageEvent::FilesDropped(vec![path.clone()])),
            ),
            (
                DragDropEvent::Drop {
                    paths: Vec::new(),
                    position: at,
                },
                None,
            ),
            (
                DragDropEvent::Enter {
                    paths: vec![path],
                    position: at,
                },
                None,
            ),
            (DragDropEvent::Leave, None),
        ];
        for (event, want) in cases {
            assert_eq!(files_dropped(event), want);
        }
    }

    const CASES: &[Case] = &[
        ("zero width is lifted to one pixel", 0.0, 30.0, 1.0, 30.0),
        ("zero height is lifted to one pixel", 40.0, 0.0, 40.0, 1.0),
        ("a normal size is kept", 40.0, 30.0, 40.0, 30.0),
    ];

    fn logical(
        width: f64,
        height: f64,
    ) -> (f64, f64, f64, f64) {
        let rect = to_rect(Bounds {
            x: 1.0,
            y: 2.0,
            width,
            height,
        });
        let size = rect.size.to_logical::<f64>(1.0);
        let at = rect.position.to_logical::<f64>(1.0);
        (size.width, size.height, at.x, at.y)
    }

    #[test]
    fn bounds_become_logical_rects_with_a_minimum_size() {
        let wrong: Vec<&str> = CASES
            .iter()
            .filter(|(_, width, height, want_width, want_height)| {
                logical(*width, *height) != (*want_width, *want_height, 1.0, 2.0)
            })
            .map(|(name, ..)| *name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
    }

    #[test]
    fn protocol_responses_keep_status_headers_and_body() {
        let response = into_http(ProtocolResponse {
            status: 201,
            headers: vec![("content-type".into(), "text/plain".into())],
            body: b"hi".to_vec(),
        });
        assert_eq!(response.status().as_u16(), 201, "status");
        assert_eq!(
            response
                .headers()
                .get("content-type")
                .map(HeaderValue::as_bytes),
            Some(&b"text/plain"[..]),
            "header"
        );
        assert_eq!(response.body().as_ref(), b"hi", "body");
    }

    #[test]
    fn invalid_headers_turn_into_a_500() {
        let response = into_http(ProtocolResponse {
            status: 200,
            headers: vec![("bad header".into(), "x".into())],
            body: Vec::new(),
        });
        assert_eq!(response.status().as_u16(), 500, "status");
    }
}
