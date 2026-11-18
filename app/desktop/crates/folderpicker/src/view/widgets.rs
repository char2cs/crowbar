use gpui::{Div, ParentElement, SharedString, Styled};
use ui::{Palette, Pose};

use crate::look::TREE_HEIGHT;
use crate::tree::Toggle;
use crate::view::View;
use crate::view::muted::muted;

pub(crate) fn centred(
    palette: &Palette,
    text: impl Into<SharedString>,
) -> Div {
    muted(palette)
        .h(TREE_HEIGHT)
        .flex()
        .items_center()
        .justify_center()
        .child(text.into())
}

pub(crate) fn highlighted(
    row: Div,
    palette: &Palette,
    chosen: Pose,
) -> Div {
    match chosen {
        Pose::On => row.bg(palette.accent()),
        Pose::Off => row,
    }
}

pub(crate) fn toggler(
    view: &gpui::Entity<View>,
    path: &str,
) -> Toggle {
    let (view, path) = (view.clone(), path.to_owned());
    Box::new(move |_, cx| {
        cx.stop_propagation();
        view.update(cx, |this, cx| this.toggle(&path, cx));
    })
}

pub(crate) fn inert() -> Toggle {
    Box::new(|_, _| {})
}

#[cfg(test)]
mod tests {
    use gpui::div;
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn only_a_chosen_row_gets_the_accent_background() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Window);
        let cases = [(Pose::On, true), (Pose::Off, false)];
        for (chosen, painted) in cases {
            let mut row = highlighted(div(), palette, chosen);
            assert_eq!(row.style().background.is_some(), painted, "{chosen:?}");
        }
    }
}
