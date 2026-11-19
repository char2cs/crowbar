use wry::WebView;

use crate::backend::native::linux::gtk_state::remembered_place::RememberedPlace;
use crate::view::Kind;

pub(crate) struct LiveView {
    pub(crate) view: WebView,
    pub(crate) kind: Kind,
    pub(crate) place: RememberedPlace,
    pub(crate) page_window: u32,
    pub(crate) parent: u32,
}
