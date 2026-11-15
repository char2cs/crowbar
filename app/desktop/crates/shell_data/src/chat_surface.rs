//! Which face of a chat is the chat itself.

/// The face a chat is driven through.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum ChatSurface {
    /// The provider's own conversation view.
    #[default]
    Provider,
    /// The terminal: its PTY is the chat.
    Terminal,
}

impl ChatSurface {
    pub(crate) fn from_wire(text: &str) -> Self {
        match text {
            "terminal" => Self::Terminal,
            _ => Self::Provider,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_word_terminal_selects_the_terminal() {
        assert_eq!(ChatSurface::from_wire("terminal"), ChatSurface::Terminal);
        assert_eq!(ChatSurface::from_wire(""), ChatSurface::Provider);
        assert_eq!(ChatSurface::from_wire("other"), ChatSurface::Provider);
    }
}
