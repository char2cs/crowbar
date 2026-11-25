use gpui::Context;

use super::Document;
use crate::{DiskTake, SaveMark};

impl Document {
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

    /// Text read from disk under `since`: a clean document takes it as saved, in one undo step
    /// that keeps every view's carets; a dirty one keeps its edits and is flagged once, when disk
    /// holds neither its saved text nor its own, as develop's `takeDiskText`.
    pub fn take_disk_text(
        &mut self,
        (disk, since): (&str, SaveMark),
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        if since != self.save_mark {
            return DiskTake::Kept;
        }
        if !self.is_dirty(cx) {
            return self.take_clean(disk, cx);
        }
        let moved = self.saved != disk && self.model.read(cx).text() != disk;
        if !moved || self.disk_conflict {
            return DiskTake::Kept;
        }
        self.disk_conflict = true;
        DiskTake::Conflicted
    }

    /// Text read from disk for a reload the user asked for under `since`: the document shows it
    /// as saved in one undo step, dropping its edits, unless a save landed after the read began.
    pub fn take_reloaded_text(
        &mut self,
        (disk, since): (&str, SaveMark),
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        if since != self.save_mark {
            return DiskTake::Kept;
        }
        self.adopt(disk, cx)
    }

    /// Text a rich view wrote: one undoable edit that keeps every view's carets, dirty against
    /// the saved text as typing is. The same text again changes nothing.
    pub fn take_page_text(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.model
            .update(cx, |model, cx| model.replace_all(text, cx));
    }

    fn take_clean(
        &mut self,
        disk: &str,
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        if self.saved == disk {
            return DiskTake::Kept;
        }
        self.adopt(disk, cx)
    }

    fn adopt(
        &mut self,
        disk: &str,
        cx: &mut Context<'_, Self>,
    ) -> DiskTake {
        self.saved = gpui_kit::base::input::Rope::from(disk);
        self.save_mark = next_mark(self.save_mark);
        self.disk_conflict = false;
        self.model
            .update(cx, |model, cx| model.replace_all(disk, cx));
        self.report_dirty(cx);
        DiskTake::Taken
    }
}

pub(super) const fn next_mark(mark: SaveMark) -> SaveMark {
    SaveMark(mark.0.wrapping_add(1))
}

#[cfg(test)]
mod tests {
    use gpui::{Entity, TestAppContext, VisualTestContext};

    use super::*;
    use crate::editor_view::test_support::{document_of, listened, opened, press};
    use crate::{Access, EditorEvent, EditorView};

    fn take(
        document: &Entity<Document>,
        (disk, since): (&str, SaveMark),
        window: &mut VisualTestContext,
    ) -> DiskTake {
        document.update(window, |document, cx| {
            document.take_disk_text((disk, since), cx)
        })
    }

    fn reload(
        document: &Entity<Document>,
        since: SaveMark,
        window: &mut VisualTestContext,
    ) -> DiskTake {
        document.update(window, |document, cx| {
            document.take_reloaded_text(("a", since), cx)
        })
    }

    fn mark(
        document: &Entity<Document>,
        window: &mut VisualTestContext,
    ) -> SaveMark {
        document.read_with(window, |document, _| document.save_mark())
    }

    fn dirty(
        document: &Entity<Document>,
        window: &mut VisualTestContext,
    ) -> bool {
        document.read_with(window, Document::is_dirty)
    }

    #[gpui::test]
    fn a_clean_document_takes_the_disk_text_in_one_undo_step_keeping_the_caret(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a\nb\nc", Some(3)), Access::Editable, cx);
        listened(&view, window);
        let document = document_of(&view, window);
        let since = mark(&document, window);
        assert_eq!(
            take(&document, ("a\nB\nc\nd", since), window),
            DiskTake::Taken
        );
        assert_eq!(view.read_with(window, EditorView::text), "a\nB\nc\nd");
        assert!(!dirty(&document, window), "clean");
        assert_eq!(view.read_with(window, |view, cx| view.caret(cx).line), 3);
        press("@input::Undo", window);
        assert_eq!(
            view.read_with(window, EditorView::text),
            "a\nb\nc",
            "undone"
        );
        assert!(dirty(&document, window), "undo leaves disk");
    }

    #[gpui::test]
    fn a_dirty_document_keeps_its_edits_and_is_flagged_once_when_disk_moves(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        listened(&view, window);
        let document = document_of(&view, window);
        window.simulate_input("x");
        let since = mark(&document, window);
        let cases = [
            ("a", DiskTake::Kept),
            ("xa", DiskTake::Kept),
            ("b", DiskTake::Conflicted),
            ("c", DiskTake::Kept),
        ];
        for (disk, expected) in cases {
            assert_eq!(take(&document, (disk, since), window), expected, "{disk}");
            assert_eq!(view.read_with(window, EditorView::text), "xa", "{disk}");
        }
    }

    #[gpui::test]
    fn a_reload_drops_the_edits_for_the_disk_text_unless_a_save_landed_first(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        listened(&view, window);
        let document = document_of(&view, window);
        window.simulate_input("x");
        let since = mark(&document, window);
        let stale = SaveMark(since.0.wrapping_add(1));
        assert_eq!(reload(&document, stale, window), DiskTake::Kept, "stale");
        assert_eq!(reload(&document, since, window), DiskTake::Taken, "taken");
        assert_eq!(view.read_with(window, EditorView::text), "a");
        assert!(!dirty(&document, window), "clean");
    }

    #[gpui::test]
    fn a_read_begun_before_the_latest_save_never_lands(cx: &mut TestAppContext) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        listened(&view, window);
        let document = document_of(&view, window);
        let since = mark(&document, window);
        window.simulate_input("x");
        document.update(window, |document, cx| document.mark_saved("xa", cx));
        assert_eq!(take(&document, ("a", since), window), DiskTake::Kept);
        assert_eq!(view.read_with(window, EditorView::text), "xa");
    }

    #[gpui::test]
    fn the_rich_view_s_text_dirties_the_document_once_and_undoes_in_one_step(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a", None), Access::Editable, cx);
        let heard = listened(&view, window);
        let document = document_of(&view, window);
        for _ in 0..2 {
            document.update(window, |document, cx| document.take_page_text("a b", cx));
        }
        assert_eq!(view.read_with(window, EditorView::text), "a b");
        assert_eq!(*heard.borrow(), [EditorEvent::DirtyChanged(true)]);
        press("@input::Undo", window);
        assert_eq!(
            view.read_with(window, EditorView::text),
            "a",
            "one undo step"
        );
    }
}
