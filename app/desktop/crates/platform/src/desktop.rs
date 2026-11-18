/// The desktop the shell runs on. Windows is not supported.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Desktop {
    /// A freedesktop desktop (`xdg-open`).
    Linux,
    /// macOS (`open`).
    MacOs,
}

impl Desktop {
    /// The desktop this binary was built for.
    #[must_use]
    pub fn current() -> Self {
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
