use gpui::{App, InteractiveElement as _, IntoElement};
use settings::Heading;

use crate::modal::pages::title_block::title_block;

pub(crate) fn heading_view(
    heading: &Heading,
    cx: &App,
) -> impl IntoElement {
    title_block(heading.title(), Some(heading.description()), cx)
        .debug_selector(|| "page-heading".to_owned())
}

#[cfg(test)]
mod tests {
    use settings::{Block, default_schema};

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_heading_is_as_tall_as_its_title_plus_its_description(cx: &mut gpui::TestAppContext) {
        let schema = default_schema().unwrap();
        let heading = schema
            .pages()
            .iter()
            .flat_map(|page| page.blocks().iter())
            .find_map(|block| match block {
                Block::Heading(heading) => Some(heading.clone()),
                _ => None,
            })
            .unwrap();
        let tall = measure(cx, move |_, cx| {
            heading_view(&heading, cx).into_any_element()
        });
        let flat = measure(cx, |_, cx| {
            title_block("Agents", None, cx).into_any_element()
        });
        assert!(tall.size.height > flat.size.height);
    }
}
