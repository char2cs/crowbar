use gpui::{App, Pixels, TextSystem, font, px};
use store::SystemFonts;

const PROBE_SIZE: Pixels = px(16.0);
const NARROW: char = 'i';
const WIDE: char = 'M';

pub(crate) fn installed(cx: &App) -> SystemFonts {
    let text = cx.text_system();
    let mut all = text.all_font_names();
    all.sort_unstable();
    all.dedup();
    let monospace = all
        .iter()
        .filter(|family| is_monospace(text, family))
        .cloned()
        .collect();
    SystemFonts { all, monospace }
}

fn is_monospace(
    text: &TextSystem,
    family: &str,
) -> bool {
    let id = text.resolve_font(&font(family.to_owned()));
    let narrow = text.advance(id, PROBE_SIZE, NARROW);
    let wide = text.advance(id, PROBE_SIZE, WIDE);
    matches!((narrow, wide), (Ok(a), Ok(b)) if a == b)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_families_are_sorted_unique_and_the_monospace_ones_are_among_them(
        cx: &mut TestAppContext
    ) {
        let fonts = cx.update(|cx| installed(cx));
        let mut ordered = fonts.all.clone();
        ordered.sort_unstable();
        ordered.dedup();
        assert_eq!(fonts.all, ordered, "sorted and unique");
        assert!(
            fonts
                .monospace
                .iter()
                .all(|family| fonts.all.contains(family)),
            "monospace is a subset"
        );
    }
}
