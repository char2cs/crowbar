use gpui::{AnyElement, Hsla, IntoElement, SharedString, px};
use ui::{Icon, IconName, ProviderGlyph};

const GLYPH: f32 = 14.0;

pub(crate) fn tab_glyph(
    icon: IconName,
    artwork: Option<SharedString>,
    ink: Hsla,
) -> AnyElement {
    let Some(artwork) = artwork else {
        return Icon::new(icon)
            .size(px(GLYPH))
            .color(ink)
            .into_any_element();
    };
    ProviderGlyph::new(artwork, ink)
        .size(px(GLYPH))
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::{Context, InteractiveElement, ParentElement, Render, TestAppContext, Window, div};
    use ui::{IconName, Theme};

    use super::*;

    struct Glyphs;

    impl Render for Glyphs {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let ink = gpui::black();
            let art = Some(SharedString::from("<svg viewBox=\"0 0 1 1\"/>"));
            div()
                .child(div().debug_selector(|| "icon".into()).child(tab_glyph(
                    IconName::File,
                    None,
                    ink,
                )))
                .child(div().debug_selector(|| "art".into()).child(tab_glyph(
                    IconName::File,
                    art,
                    ink,
                )))
        }
    }

    #[gpui::test]
    fn a_tab_glyph_is_fourteen_pixels_whether_icon_or_artwork(cx: &mut TestAppContext) {
        cx.update(|cx| {
            ui::init(cx).expect("ui init");
            cx.set_global(Theme::default());
        });
        let (_, cx) = cx.add_window_view(|_, _| Glyphs);
        cx.run_until_parked();
        for name in ["icon", "art"] {
            let height = cx.debug_bounds(name).map(|bounds| bounds.size.height);
            assert_eq!(height, Some(px(GLYPH)), "{name}");
        }
    }
}
