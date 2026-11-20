use std::collections::HashMap;
use std::sync::Arc;

use gpui::{App, Global, Image, ImageFormat, RenderImage};

#[derive(Default)]
pub(crate) struct GlyphImages {
    drawn: HashMap<String, Arc<RenderImage>>,
}

impl Global for GlyphImages {}

impl GlyphImages {
    pub(crate) fn drawn(
        svg: &str,
        cx: &mut App,
    ) -> Option<Arc<RenderImage>> {
        if let Some(found) = cx.default_global::<Self>().drawn.get(svg) {
            return Some(Arc::clone(found));
        }
        let artwork = Image::from_bytes(ImageFormat::Svg, svg.as_bytes().to_vec());
        let image = artwork
            .to_image_data(cx.svg_renderer())
            .inspect_err(|err| tracing::warn!("provider artwork: {err}"))
            .ok()?;
        let drawn = &mut cx.default_global::<Self>().drawn;
        drawn.insert(svg.to_owned(), Arc::clone(&image));
        Some(image)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    const MARK: &str = r#"<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M4 4h16v3H4z"/></svg>"#;

    #[gpui::test]
    fn artwork_is_ready_on_its_first_draw_and_kept_for_the_next(cx: &mut TestAppContext) {
        let (first, again) =
            cx.update(|cx| (GlyphImages::drawn(MARK, cx), GlyphImages::drawn(MARK, cx)));
        let kept = first.filter(|first| again.is_some_and(|again| Arc::ptr_eq(first, &again)));
        assert!(kept.is_some(), "drawn on the first frame and decoded once");
    }
}
