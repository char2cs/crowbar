//! A page heading.

use gpui::{App, IntoElement};

use crate::heading::Heading;
use crate::title_block::title_block;

pub(crate) fn heading_view(heading: &Heading, cx: &App) -> impl IntoElement {
    title_block(&heading.title, Some(&heading.description), cx)
}

#[cfg(test)]
#[path = "heading_view_tests.rs"]
mod tests;
