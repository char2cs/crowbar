//! The dialog's body: header, rail, pages, notices, key routing and the shortcut capture view.

mod body;
mod empty_view;
mod header;
mod key_routing;
mod notice_view;
mod page_picker;
pub(crate) mod pages;
mod prompt_view;
mod rail;

pub(crate) use body::body;
pub(crate) use header::{extent, headed, is_narrow, search_field};
pub(crate) use notice_view::notice_view;
pub(crate) use prompt_view::{ACCEPTED, prompt_request, prompt_view};
