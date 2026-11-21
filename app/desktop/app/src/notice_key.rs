use ui::ToastLifetime;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum NoticeKey {
    Layout,
    Storage,
    Preferences,
    Daemon,
    Window,
    Command,
    Terminal,
}

impl NoticeKey {
    pub(crate) fn slug(self) -> &'static str {
        match self {
            Self::Layout => "layout",
            Self::Storage => "storage",
            Self::Preferences => "preferences",
            Self::Daemon => "daemon",
            Self::Window => "window",
            Self::Command => "command",
            Self::Terminal => "terminal",
        }
    }

    pub(crate) fn lifetime(self) -> ToastLifetime {
        match self {
            Self::Daemon | Self::Window | Self::Storage | Self::Preferences => {
                ToastLifetime::Pinned
            },
            Self::Layout | Self::Command | Self::Terminal => ToastLifetime::Auto,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn conditions_that_outlast_a_timeout_stay_until_cleared() {
        const CASES: [(NoticeKey, &str, ToastLifetime); 7] = [
            (NoticeKey::Layout, "layout", ToastLifetime::Auto),
            (NoticeKey::Storage, "storage", ToastLifetime::Pinned),
            (NoticeKey::Preferences, "preferences", ToastLifetime::Pinned),
            (NoticeKey::Daemon, "daemon", ToastLifetime::Pinned),
            (NoticeKey::Window, "window", ToastLifetime::Pinned),
            (NoticeKey::Command, "command", ToastLifetime::Auto),
            (NoticeKey::Terminal, "terminal", ToastLifetime::Auto),
        ];
        for (key, slug, lifetime) in CASES {
            assert_eq!((key.slug(), key.lifetime()), (slug, lifetime));
        }
    }
}
