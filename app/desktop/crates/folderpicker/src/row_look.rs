//! Sizes and strings of the remote folder picker.

use gpui::{Pixels, px};

pub(crate) const TREE_HEIGHT: Pixels = px(240.0);
pub(crate) const ROW_HEIGHT: Pixels = px(28.0);
pub(crate) const INDENT: Pixels = px(14.0);
pub(crate) const GAP: Pixels = px(6.0);
pub(crate) const ICON: Pixels = px(14.0);
pub(crate) const TEXT: Pixels = px(13.0);
pub(crate) const INERT_OPACITY: f32 = 0.4;
pub(crate) const BANNER_TINT: f32 = 0.1;

pub(crate) const TITLE: &str = "Choose a folder";
pub(crate) const EMPTY_ROOT: &str = "This folder is empty.";
pub(crate) const LOADING_ROOT: &str = "Loading folders…";
