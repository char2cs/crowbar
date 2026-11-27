use gpui::{AnyElement, InteractiveElement, IntoElement, ParentElement, Styled, div, px};
use ui::{Icon, IconName, Palette};

use crate::tabbar::look::tab_access::TabAccess;
use crate::tabbar::look::tab_look::TabLook;
use crate::tabbar::tab::tab_state::TabState;

const DOT: f32 = 8.0;
const LOCK: f32 = 12.0;

pub(crate) fn after_label(
    look: &TabLook,
    state: TabState,
    palette: &Palette,
) -> Vec<AnyElement> {
    let lock = (look.access == TabAccess::ReadOnly).then(|| lock_of(palette));
    let dot = (look.dirty && state == TabState::Idle).then(|| unsaved_dot(palette));
    lock.into_iter().chain(dot).collect()
}

fn lock_of(palette: &Palette) -> AnyElement {
    div()
        .flex_none()
        .debug_selector(|| "strip-tab-lock".into())
        .child(
            Icon::new(IconName::Lock)
                .size(px(LOCK))
                .color(palette.muted_foreground()),
        )
        .into_any_element()
}

fn unsaved_dot(palette: &Palette) -> AnyElement {
    div()
        .flex_none()
        .size(px(DOT))
        .rounded_full()
        .bg(palette.primary())
        .debug_selector(|| "strip-tab-dot".into())
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn a_lock_follows_a_read_only_label_and_a_dot_an_unshown_unsaved_one() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Window);
        let file = TabLook::plain("main.rs");
        let cases = [
            ("a plain tab", file.clone(), TabState::Idle, 0),
            ("read-only", file.clone().read_only(), TabState::Active, 1),
            (
                "unsaved and hidden",
                file.clone().unsaved(),
                TabState::Idle,
                1,
            ),
            (
                "unsaved and shown",
                file.clone().unsaved(),
                TabState::Active,
                0,
            ),
            ("both", file.read_only().unsaved(), TabState::Idle, 2),
        ];
        for (name, look, state, expected) in cases {
            assert_eq!(after_label(&look, state, palette).len(), expected, "{name}");
        }
    }
}
