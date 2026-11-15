//! The toasts the sidebar overlay holds: keyed replacement and the three newest shown.

use ui::ToastKind;

use crate::toast_entry::ToastEntry;

const VISIBLE: usize = 3;

#[derive(Debug, Default)]
pub(crate) struct ToastStack {
    entries: Vec<ToastEntry>,
    next: u64,
}

impl ToastStack {
    pub(crate) fn push(
        &mut self,
        (kind, message): (ToastKind, String),
        key: Option<&str>,
    ) -> u64 {
        let id = self.next;
        self.next += 1;
        let replaced = key.and_then(|key| {
            let at = self
                .entries
                .iter()
                .position(|entry| entry.key.as_deref() == Some(key))?;
            Some(self.entries.remove(at).updates + 1)
        });
        self.entries.push(ToastEntry {
            id,
            key: key.map(str::to_owned),
            kind,
            message,
            updates: replaced.unwrap_or(0),
        });
        id
    }

    pub(crate) fn expire(
        &mut self,
        id: u64,
    ) {
        self.entries.retain(|entry| entry.id != id);
    }

    pub(crate) fn visible(&self) -> &[ToastEntry] {
        let from = self.entries.len().saturating_sub(VISIBLE);
        self.entries.get(from..).unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Push = (&'static str, Option<&'static str>);
    type Case = (&'static str, &'static [Push], &'static [&'static str]);

    fn shown_after(pushes: &[Push]) -> Vec<String> {
        let mut stack = ToastStack::default();
        for (message, key) in pushes {
            stack.push((ToastKind::Info, (*message).to_owned()), *key);
        }
        stack
            .visible()
            .iter()
            .map(|entry| entry.message.clone())
            .collect()
    }

    #[test]
    fn pushes_show_the_three_newest_and_a_key_replaces_its_predecessor() {
        const CASES: [Case; 3] = [
            (
                "four plain toasts",
                &[("a", None), ("b", None), ("c", None), ("d", None)],
                &["b", "c", "d"],
            ),
            (
                "a keyed toast",
                &[
                    ("Reconnecting", Some("connection")),
                    ("other", None),
                    ("Reconnected", Some("connection")),
                ],
                &["other", "Reconnected"],
            ),
            ("nothing pushed", &[], &[]),
        ];
        for (name, pushes, expected) in CASES {
            assert_eq!(shown_after(pushes), expected, "{name}");
        }
    }

    #[test]
    fn expiring_an_id_removes_only_that_toast() {
        let mut stack = ToastStack::default();
        let first = stack.push((ToastKind::Info, "a".into()), None);
        stack.push((ToastKind::Info, "b".into()), None);
        stack.expire(first);
        stack.expire(first);
        let shown: Vec<&str> = stack.visible().iter().map(|e| e.message.as_str()).collect();
        assert_eq!(shown, ["b"]);
    }
}
