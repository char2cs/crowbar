//! How long a toast stays.

use std::time::Duration;

use crate::display::toast::toast_kind::ToastKind;
use crate::foundation::tokens::timing;

/// Whether a toast leaves on its own.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ToastLifetime {
    /// Dismissed after five seconds; a loading toast never expires.
    #[default]
    Auto,
    /// Stays until it is dismissed (the React `timeout: 0` toast).
    Pinned,
}

impl ToastLifetime {
    pub(crate) fn timeout(
        self,
        kind: ToastKind,
    ) -> Option<Duration> {
        if self == Self::Pinned || kind == ToastKind::Loading {
            return None;
        }
        Some(timing::TOAST_TIMEOUT)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_pinned_and_loading_toasts_never_expire() {
        const CASES: &[(ToastLifetime, ToastKind, bool)] = &[
            (ToastLifetime::Auto, ToastKind::Info, true),
            (ToastLifetime::Auto, ToastKind::Error, true),
            (ToastLifetime::Auto, ToastKind::Loading, false),
            (ToastLifetime::Pinned, ToastKind::Error, false),
        ];
        for &(lifetime, kind, expires) in CASES {
            assert_eq!(
                lifetime.timeout(kind).is_some(),
                expires,
                "{lifetime:?} {kind:?}"
            );
        }
    }
}
