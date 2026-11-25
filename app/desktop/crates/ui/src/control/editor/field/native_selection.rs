//! The fill under selected text in a field, as the webview draws it for develop's inputs.

use gpui::{Hsla, rgba};

// PLATFORM: linux WebKitGTK draws Adwaita's accent at 30% since develop sets no selection colour.
const WEBKIT_GTK_SELECTION: u32 = 0x3584_e44d;

pub(crate) fn native_selection() -> Hsla {
    rgba(WEBKIT_GTK_SELECTION).into()
}

#[cfg(test)]
mod tests {
    use gpui::Rgba;

    use super::*;

    const ROW: [f32; 3] = [43.0, 42.0, 40.0];
    const TAURI: [f32; 3] = [46.0, 69.0, 96.0];

    #[test]
    fn over_a_hovered_row_it_draws_the_blue_tauri_draws() {
        let fill = Rgba::from(native_selection());
        let ink = [fill.r, fill.g, fill.b].map(|channel| channel * 255.0);
        for ((row, ink), tauri) in ROW.iter().zip(ink).zip(TAURI) {
            let blended = row + (ink - row) * fill.a;
            assert!((blended - tauri).abs() <= 1.5, "{blended} vs {tauri}");
        }
    }
}
