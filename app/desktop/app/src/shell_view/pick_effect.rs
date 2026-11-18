use browser::{PageUpdate, pick_count, pointer_text};
use ui::ToastKind;

const COPIED: &str = "Pointer copied";

#[derive(Debug, PartialEq, Eq)]
pub(super) struct PickEffect {
    pub(super) copy: Option<String>,
    pub(super) notice: Option<(ToastKind, String)>,
}

impl PickEffect {
    pub(super) fn of(
        update: &PageUpdate,
        tab: &str,
    ) -> Self {
        match update {
            PageUpdate::Picked { report } => Self {
                copy: Some(pointer_text(report, tab)),
                notice: Some((ToastKind::Success, copied_text(pick_count(report)))),
            },
            PageUpdate::PickFailed { message } => Self {
                copy: None,
                notice: Some((ToastKind::Error, format!("Could not pick: {message}"))),
            },
            _ => Self {
                copy: None,
                notice: None,
            },
        }
    }
}

fn copied_text(count: usize) -> String {
    if count > 1 {
        return format!("{COPIED}: {count} items");
    }
    COPIED.to_owned()
}

#[cfg(test)]
mod tests {
    use browser::PickReport;

    use super::*;

    fn found(more: usize) -> PageUpdate {
        let report = PickReport {
            kind: "region".into(),
            title: "T".into(),
            url: "https://a.test/".into(),
            items: Vec::new(),
            more,
            bounds: vec![0, 0, 1, 1],
            quote: None,
        };
        PageUpdate::Picked { report }
    }

    #[test]
    fn a_finished_pick_copies_its_pointer_and_a_failed_one_only_says_so() {
        let cases = [
            (
                found(2),
                Some("tab p1"),
                Some((ToastKind::Success, "Pointer copied: 2 items")),
            ),
            (
                found(1),
                Some("tab p1"),
                Some((ToastKind::Success, "Pointer copied")),
            ),
            (
                PageUpdate::PickFailed {
                    message: "boom".into(),
                },
                None,
                Some((ToastKind::Error, "Could not pick: boom")),
            ),
            (PageUpdate::PickEnded, None, None),
        ];
        for (update, copied, notice) in cases {
            let effect = PickEffect::of(&update, "p1");
            let copy_matches = match (&effect.copy, copied) {
                (Some(text), Some(part)) => text.contains(part),
                (None, None) => true,
                _ => false,
            };
            assert!(copy_matches, "{update:?}: {:?}", effect.copy);
            let seen = effect
                .notice
                .as_ref()
                .map(|(kind, text)| (*kind, text.as_str()));
            assert_eq!(seen, notice, "{update:?}");
        }
    }
}
