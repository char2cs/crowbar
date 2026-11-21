use sidebar::Error;
use ui::{ToastContent, ToastKind};

pub(crate) fn refusal_toast(error: Error) -> Option<ToastContent> {
    match error {
        Error::Refused { reason } => Some(ToastContent::new(ToastKind::Error, reason)),
        Error::Failed { title, reason } => {
            Some(ToastContent::new(ToastKind::Error, reason).titled(title))
        },
        other => {
            tracing::warn!(error = %other, "sidebar command failed");
            None
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_failed_command_is_said_under_its_title_and_a_bare_refusal_without_one() {
        let failed = Error::Failed {
            title: "Failed to update lock".into(),
            reason: "this row is not loaded any more".into(),
        };
        let refused = Error::Refused {
            reason: "busy".into(),
        };
        let titled = ToastContent::new(ToastKind::Error, "this row is not loaded any more")
            .titled("Failed to update lock");
        assert_eq!(refusal_toast(failed), Some(titled));
        assert_eq!(
            refusal_toast(refused),
            Some(ToastContent::new(ToastKind::Error, "busy"))
        );
    }
}
