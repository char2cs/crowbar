use ui::ToastLifetime;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum NoticeKey {
    Layout,
    Daemon,
    Window,
    Command,
}

impl NoticeKey {
    pub(crate) fn slug(self) -> &'static str {
        match self {
            Self::Layout => "layout",
            Self::Daemon => "daemon",
            Self::Window => "window",
            Self::Command => "command",
        }
    }

    pub(crate) fn lifetime(self) -> ToastLifetime {
        match self {
            Self::Daemon | Self::Window => ToastLifetime::Pinned,
            Self::Layout | Self::Command => ToastLifetime::Auto,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn conditions_that_outlast_a_timeout_stay_until_cleared() {
        const CASES: [(NoticeKey, &str, ToastLifetime); 4] = [
            (NoticeKey::Layout, "layout", ToastLifetime::Auto),
            (NoticeKey::Daemon, "daemon", ToastLifetime::Pinned),
            (NoticeKey::Window, "window", ToastLifetime::Pinned),
            (NoticeKey::Command, "command", ToastLifetime::Auto),
        ];
        for (key, slug, lifetime) in CASES {
            assert_eq!((key.slug(), key.lifetime()), (slug, lifetime));
        }
    }
}
