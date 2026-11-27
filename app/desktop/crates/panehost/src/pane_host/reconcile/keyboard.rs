#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Keyboard {
    #[default]
    Pane,
    Shell,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_page_has_the_keyboard_until_the_shell_asks_for_it() {
        assert_eq!(Keyboard::default(), Keyboard::Pane, "default");
    }
}
