#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Desktop {
    Linux,
    MacOs,
}

impl Desktop {
    #[must_use]
    pub(crate) fn current() -> Self {
        if cfg!(target_os = "macos") {
            return Self::MacOs;
        }
        Self::Linux
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_current_desktop_follows_the_build_target() {
        assert_eq!(
            Desktop::current() == Desktop::MacOs,
            cfg!(target_os = "macos")
        );
    }
}
