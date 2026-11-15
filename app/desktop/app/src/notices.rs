use gpui::SharedString;
use ui::ToastKind;

use crate::notice_key::NoticeKey;

#[derive(Debug, Default)]
pub(crate) struct Notices {
    entries: Vec<(NoticeKey, ToastKind, SharedString)>,
}

impl Notices {
    pub(crate) fn set(
        &mut self,
        key: NoticeKey,
        kind: ToastKind,
        text: impl Into<SharedString>,
    ) {
        self.clear(key);
        self.entries.push((key, kind, text.into()));
    }

    pub(crate) fn clear(
        &mut self,
        key: NoticeKey,
    ) {
        self.entries.retain(|(known, ..)| *known != key);
    }

    pub(crate) fn iter(&self) -> impl Iterator<Item = &(NoticeKey, ToastKind, SharedString)> {
        self.entries.iter()
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::notice_key::NoticeKey;

    #[test]
    fn a_subject_has_one_notice_and_clearing_removes_it() {
        let mut notices = Notices::default();
        notices.set(NoticeKey::Daemon, ToastKind::Error, "down");
        notices.set(NoticeKey::Daemon, ToastKind::Error, "still down");
        notices.set(NoticeKey::Layout, ToastKind::Warning, "fresh start");
        let texts: Vec<&str> = notices.iter().map(|(.., text)| text.as_ref()).collect();
        assert_eq!(
            texts,
            ["still down", "fresh start"],
            "replaced, not stacked"
        );
        notices.clear(NoticeKey::Daemon);
        assert_eq!(notices.iter().count(), 1, "one left");
        notices.clear(NoticeKey::Layout);
        assert!(notices.is_empty(), "none left");
    }
}
