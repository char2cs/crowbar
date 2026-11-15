use wry::WebView;

use crate::web_view_kind::WebViewKind;

pub(crate) struct LiveView {
    pub(crate) view: WebView,
    pub(crate) kind: WebViewKind,
}
