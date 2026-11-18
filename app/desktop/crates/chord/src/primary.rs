//! Which key plays the part of `mod` in a chord.

/// The primary shortcut modifier of the platform: a closed set, so other crates match it fully.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Primary {
    /// Control: Linux.
    Control,
    /// Command: macOS.
    Platform,
}

impl Primary {
    /// The primary modifier of the platform this build runs on.
    pub fn native() -> Self {
        if cfg!(target_os = "macos") {
            return Self::Platform;
        }
        Self::Control
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn linux_uses_control() {
        if cfg!(target_os = "linux") {
            assert_eq!(Primary::native(), Primary::Control);
        }
    }
}
