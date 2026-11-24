use gpui::{Context, Window};
use gpui_kit::base::input::Rope;

use super::EditorView;
use crate::{DiskTake, SaveMark};

impl EditorView {
    /// The mark of the latest open or save; a disk read carries it to [`Self::take_disk_text`].
    #[must_use]
    pub const fn save_mark(&self) -> SaveMark {
        self.save_mark
    }

    /// The text last saved (or opened) that dirtiness is measured against.
    #[must_use]
    pub fn saved_text(&self) -> String {
        self.saved.to_string()
    }

    /// Text read from disk under `since`: a clean view takes it as saved, in one undo step that
    /// keeps the carets; a dirty one keeps its edits and is flagged once, when disk holds
    /// neither its saved text nor its own, as develop's `takeDiskText`.
    pub fn take_disk_text(
        &mut self,
        (disk, since): (&str, SaveMark),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        if since != self.save_mark {
            return DiskTake::Kept;
        }
        if !self.is_dirty(cx) {
            return self.take_clean(disk, window, cx);
        }
        let text = self.state.read(cx).text().clone();
        let moved = self.saved != disk && text != disk;
        if !moved || self.disk_conflict {
            return DiskTake::Kept;
        }
        self.disk_conflict = true;
        DiskTake::Conflicted
    }

    /// Text read from disk for a reload the user asked for under `since`: the view shows it as
    /// saved in one undo step, dropping its edits, unless a save landed after the read began.
    pub fn take_reloaded_text(
        &mut self,
        (disk, since): (&str, SaveMark),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        if since != self.save_mark {
            return DiskTake::Kept;
        }
        self.adopt(disk, window, cx)
    }

    fn take_clean(
        &mut self,
        disk: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        if self.saved == disk {
            return DiskTake::Kept;
        }
        self.adopt(disk, window, cx)
    }

    fn adopt(
        &mut self,
        disk: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        self.saved = Rope::from(disk);
        self.save_mark = next_mark(self.save_mark);
        self.disk_conflict = false;
        self.state.update(cx, |state, cx| {
            state.replace_all_keeping_selections(disk.to_owned(), window, cx);
        });
        DiskTake::Taken
    }
}

pub(super) const fn next_mark(mark: SaveMark) -> SaveMark {
    SaveMark(mark.0.wrapping_add(1))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::Access;
    use crate::editor_view::test_support::{listened, opened};

    fn take(
        view: &gpui::Entity<EditorView>,
        (disk, since): (&str, SaveMark),
        window: &mut gpui::VisualTestContext,
    ) -> DiskTake {
        view.update_in(window, |view, window, cx| {
            view.take_disk_text((disk, since), window, cx)
        })
    }

    fn reload(
        view: &gpui::Entity<EditorView>,
        since: SaveMark,
        window: &mut gpui::VisualTestContext,
    ) -> DiskTake {
        view.update_in(window, |view, window, cx| {
            view.take_reloaded_text(("a", since), window, cx)
        })
    }

    fn mark(
        view: &gpui::Entity<EditorView>,
        window: &mut gpui::VisualTestContext,
    ) -> SaveMark {
        view.read_with(window, |view, _| view.save_mark())
    }

    #[gpui::test]
    fn a_clean_view_takes_the_disk_text_in_one_undo_step_keeping_its_caret(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a\nb\nc", Some(3)), Access::Editable, cx);
        listened(&view, window);
        let since = mark(&view, window);
        assert_eq!(take(&view, ("a\nB\nc\nd", since), window), DiskTake::Taken);
        assert_eq!(view.read_with(window, EditorView::text), "a\nB\nc\nd");
        assert!(!view.read_with(window, EditorView::is_dirty), "clean");
        assert_eq!(view.read_with(window, |view, cx| view.caret(cx).line), 3);
        window.simulate_keystrokes("ctrl-z");
        assert_eq!(
            view.read_with(window, EditorView::text),
            "a\nb\nc",
            "undone"
        );
        assert!(
            view.read_with(window, EditorView::is_dirty),
            "undo leaves disk"
        );
    }

    #[gpui::test]
    fn a_dirty_view_keeps_its_edits_and_is_flagged_once_when_disk_moves(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        listened(&view, window);
        window.simulate_input("x");
        let since = mark(&view, window);
        let cases = [
            ("a", DiskTake::Kept),
            ("xa", DiskTake::Kept),
            ("b", DiskTake::Conflicted),
            ("c", DiskTake::Kept),
        ];
        for (disk, expected) in cases {
            assert_eq!(take(&view, (disk, since), window), expected, "{disk}");
            assert_eq!(view.read_with(window, EditorView::text), "xa", "{disk}");
        }
    }

    #[gpui::test]
    fn a_reload_drops_the_edits_for_the_disk_text_unless_a_save_landed_first(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        listened(&view, window);
        window.simulate_input("x");
        let since = mark(&view, window);
        let stale = SaveMark(since.0.wrapping_add(1));
        assert_eq!(reload(&view, stale, window), DiskTake::Kept, "stale");
        assert_eq!(reload(&view, since, window), DiskTake::Taken, "taken");
        assert_eq!(view.read_with(window, EditorView::text), "a");
        assert!(!view.read_with(window, EditorView::is_dirty), "clean");
    }

    #[gpui::test]
    fn a_read_begun_before_the_latest_save_never_lands(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        listened(&view, window);
        let since = mark(&view, window);
        window.simulate_input("x");
        window.update(|_, cx| view.update(cx, |view, cx| view.mark_saved("xa", cx)));
        assert_eq!(take(&view, ("a", since), window), DiskTake::Kept);
        assert_eq!(view.read_with(window, EditorView::text), "xa");
    }
}
