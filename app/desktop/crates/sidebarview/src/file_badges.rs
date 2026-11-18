use gpui::{Div, ParentElement, Pixels, Styled, div, px};
use sidebar::GitStatus;
use ui::Palette;

use crate::scale::{SPACE_XS, TEXT_CAPTION};
use crate::tree_look::TreeLook;

const TIGHT_COUNT_GAP: f32 = 2.0;

pub(crate) fn status_color(
    status: GitStatus,
    palette: &Palette,
) -> gpui::Hsla {
    match status {
        GitStatus::Added => palette.git_added(),
        GitStatus::Untracked => palette.git_untracked(),
        GitStatus::Modified => palette.git_modified(),
        GitStatus::Deleted => palette.git_deleted(),
        GitStatus::Clean | GitStatus::Ignored => palette.foreground(),
    }
}

pub(crate) fn letter_badge(letter: char) -> Div {
    div()
        .flex_none()
        .text_size(TEXT_CAPTION)
        .child(letter.to_string())
}

pub(crate) fn count_gap(look: &TreeLook) -> Pixels {
    if look.compact_badges {
        return px(TIGHT_COUNT_GAP);
    }
    SPACE_XS
}

pub(crate) fn counts_view(
    counts: (u32, u32),
    spacing: (&'static str, Pixels),
    palette: &Palette,
) -> Div {
    let ((added, deleted), (mono, gap)) = (counts, spacing);
    div()
        .flex()
        .gap(gap)
        .font_family(mono)
        .text_size(TEXT_CAPTION)
        .child(
            div()
                .text_color(palette.sidebar_count_added())
                .child(format!("+{added}")),
        )
        .child(
            div()
                .text_color(palette.sidebar_count_deleted())
                .child(format!("-{deleted}")),
        )
}

#[cfg(test)]
mod tests {
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn compact_badges_tighten_the_gap_between_the_counts() {
        let loose = TreeLook::default();
        let tight = TreeLook {
            compact_badges: true,
            ..TreeLook::default()
        };
        assert_eq!(count_gap(&loose), SPACE_XS, "loose");
        assert_eq!(count_gap(&tight), px(TIGHT_COUNT_GAP), "tight");
    }

    #[test]
    fn a_modified_file_is_not_drawn_in_the_foreground_colour() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Chrome);
        let modified = status_color(GitStatus::Modified, palette);
        assert_ne!(modified, status_color(GitStatus::Clean, palette));
    }
}
