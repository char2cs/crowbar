#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Primary {
    Control,
    Platform,
}

impl Primary {
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
