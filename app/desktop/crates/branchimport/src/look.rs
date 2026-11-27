use gpui::{Pixels, px};

pub(crate) const ROW_HEIGHT: Pixels = px(30.0);
pub(crate) const LIST_HEIGHT: Pixels = px(320.0);
pub(crate) const GAP: Pixels = px(8.0);
pub(crate) const ICON: Pixels = px(12.0);
pub(crate) const TEXT: Pixels = px(12.0);
pub(crate) const UNAVAILABLE_OPACITY: f32 = 0.4;

pub(crate) const TITLE: &str = "Import branches";
pub(crate) const LEAD: &str = "Bring remote branches into Crowbar as workspaces.";
pub(crate) const PLACEHOLDER: &str = "Search branches…";
pub(crate) const LOADING: &str = "Fetching branches from the remote…";
pub(crate) const FAILED: &str = "Couldn't reach the remote to list branches.";
pub(crate) const NONE_ON_REMOTE: &str = "No branches on the remote.";
pub(crate) const NONE_MATCH: &str = "No branches match that search.";
