mod body;
mod empty_view;
mod header;
mod key_routing;
mod notice_view;
mod page_picker;
pub(crate) mod pages;
mod rail;

pub(crate) use body::body;
pub(crate) use header::{extent, headed, is_narrow, search_field};
pub(crate) use notice_view::notice_view;
