/// Who the keys go to while the focused pane is shown: its page, or the shell's own widgets such
/// as a pane strip. The shell decides; the host never takes the keyboard from the shell.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Keyboard {
    /// The focused pane's page takes native focus when it becomes active.
    #[default]
    Pane,
    /// The shell keeps native focus, so a strip can read arrow keys while a page is shown.
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
