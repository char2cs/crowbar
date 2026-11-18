//! A page heading.

use gpui::{App, IntoElement};
use settings::Heading;

use crate::title_block::title_block;

pub(crate) fn heading_view(
    heading: &Heading,
    cx: &App,
) -> impl IntoElement {
    title_block(&heading.title, Some(&heading.description), cx)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_heading_is_as_tall_as_its_title_plus_its_description(cx: &mut gpui::TestAppContext) {
        let tall = measure(cx, |_, cx| {
            let heading = Heading::new("Agents", "Why.");
            heading_view(&heading, cx).into_any_element()
        });
        let flat = measure(cx, |_, cx| {
            title_block("Agents", None, cx).into_any_element()
        });
        assert!(tall.size.height > flat.size.height);
    }
}
