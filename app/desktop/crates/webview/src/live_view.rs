use wry::WebView;

use crate::remembered_place::RememberedPlace;
use crate::web_view_kind::WebViewKind;

pub(crate) struct LiveView {
    pub(crate) view: WebView,
    pub(crate) kind: WebViewKind,
    pub(crate) place: RememberedPlace,
}
