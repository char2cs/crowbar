//! One toast held by a queue.

use gpui::{SharedString, Task};

use crate::toast_id::ToastId;
use crate::toast_kind::ToastKind;
use crate::toast_lifetime::ToastLifetime;

pub(crate) struct ToastEntry {
    pub(crate) id: ToastId,
    pub(crate) key: Option<SharedString>,
    pub(crate) kind: ToastKind,
    pub(crate) message: SharedString,
    pub(crate) lifetime: ToastLifetime,
    pub(crate) updates: u32,
    pub(crate) leaving: bool,
    pub(crate) timer: Option<Task<()>>,
}

impl ToastEntry {
    pub(crate) fn is_pinned(&self) -> bool {
        self.lifetime == ToastLifetime::Pinned
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn entry(lifetime: ToastLifetime) -> ToastEntry {
        ToastEntry {
            id: ToastId(0),
            key: None,
            kind: ToastKind::Info,
            message: "m".into(),
            lifetime,
            updates: 0,
            leaving: false,
            timer: None,
        }
    }

    #[test]
    fn only_a_pinned_lifetime_is_pinned() {
        const CASES: &[(ToastLifetime, bool)] =
            &[(ToastLifetime::Auto, false), (ToastLifetime::Pinned, true)];
        for &(lifetime, pinned) in CASES {
            assert_eq!(entry(lifetime).is_pinned(), pinned, "{lifetime:?}");
        }
    }
}
