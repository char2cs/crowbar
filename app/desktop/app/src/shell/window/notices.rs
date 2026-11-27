use protocol::{PaneId, PaneToast, ToastTone};
use ui::{ToastContent, ToastKind, ToastLifetime};

const PANE_KEY_PREFIX: &str = "pane:";

pub(crate) fn pane_toast_key(
    pane: &PaneId,
    key: &str,
) -> String {
    format!("{PANE_KEY_PREFIX}{pane}:{key}")
}

pub(crate) fn pane_of_toast_key(key: &str) -> Option<&str> {
    let rest = key.strip_prefix(PANE_KEY_PREFIX)?;
    rest.split_once(':').map(|(pane, _)| pane)
}

pub(crate) fn pane_toast_content(toast: &PaneToast) -> (ToastContent, ToastLifetime) {
    let kind = kind_of(toast.tone);
    let content = match &toast.description {
        Some(detail) => ToastContent::new(kind, detail.to_string()).titled(toast.title.to_string()),
        None => ToastContent::new(kind, toast.title.to_string()),
    };
    if toast.pinned {
        return (content, ToastLifetime::Pinned);
    }
    (content, ToastLifetime::Auto)
}

pub(crate) fn failure_notice(
    title: String,
    reason: String,
) -> (String, (ToastContent, ToastLifetime)) {
    let key = format!("failed:{title}");
    let content = ToastContent::new(ToastKind::Error, reason).titled(title);
    (key, (content, ToastLifetime::Auto))
}

const fn kind_of(tone: ToastTone) -> ToastKind {
    match tone {
        ToastTone::Info => ToastKind::Info,
        ToastTone::Success => ToastKind::Success,
        ToastTone::Warning => ToastKind::Warning,
        ToastTone::Error => ToastKind::Error,
        ToastTone::Loading => ToastKind::Loading,
    }
}

#[cfg(test)]
mod tests {
    use protocol::DisplayText;

    use super::*;

    fn toast(description: Option<&str>) -> PaneToast {
        PaneToast {
            key: DisplayText::new("k").unwrap(),
            tone: ToastTone::Error,
            title: DisplayText::new("Save the file").unwrap(),
            description: description.map(|text| DisplayText::new(text).unwrap()),
            pinned: description.is_none(),
        }
    }

    #[test]
    fn a_pane_toast_keeps_its_title_over_its_detail_and_its_pinning() {
        let detailed = ToastContent::new(ToastKind::Error, "disk full").titled("Save the file");
        let plain = ToastContent::new(ToastKind::Error, "Save the file");
        let cases = [
            (toast(Some("disk full")), (detailed, ToastLifetime::Auto)),
            (toast(None), (plain, ToastLifetime::Pinned)),
        ];
        for (given, expected) in cases {
            assert_eq!(pane_toast_content(&given), expected, "{given:?}");
        }
    }

    #[test]
    fn a_failure_is_titled_for_its_command_and_keyed_so_different_ones_stack() {
        let (key, (content, lifetime)) = failure_notice("Paste failed".into(), "busy".into());
        assert_eq!(
            content,
            ToastContent::new(ToastKind::Error, "busy").titled("Paste failed")
        );
        assert_eq!(lifetime, ToastLifetime::Auto);
        let (other, _) = failure_notice("Rename failed".into(), "busy".into());
        assert_ne!(key, other);
    }

    #[test]
    fn a_pane_toast_key_names_its_pane() {
        let pane = PaneId::new("p1").unwrap();
        let key = pane_toast_key(&pane, "Save: the file");
        assert_eq!(pane_of_toast_key(&key), Some("p1"));
        assert_eq!(pane_of_toast_key("daemon"), None, "a shell notice");
    }
}
