use ui::{ToastContent, ToastLifetime};

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum NoticeAct {
    Keep,
    Settle,
    Dismiss,
    Show(ToastContent, ToastLifetime),
    Renew(ToastContent, ToastLifetime),
    WarnLater,
}
