#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) enum InputMode {
    #[default]
    Pointer,
    Keyboard,
}

impl InputMode {
    pub(crate) fn rings(self) -> bool {
        self == Self::Keyboard
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_keyboard_draws_a_focus_ring() {
        let cases = [
            ("a pointer press", InputMode::Pointer, false),
            ("a key", InputMode::Keyboard, true),
        ];
        for (name, mode, rings) in cases {
            assert_eq!(mode.rings(), rings, "{name}");
        }
        assert_eq!(InputMode::default(), InputMode::Pointer, "nothing used yet");
    }
}
