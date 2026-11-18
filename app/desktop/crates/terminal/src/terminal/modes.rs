//! What the program asked of input and the screen, and the bytes that answer it.

use alacritty_terminal::term::TermMode;

use crate::input::CursorKeys;
use crate::input::KeyInput;
use crate::input::MouseEncoding;
use crate::input::MouseEvent;
use crate::input::MouseTracking;
use crate::input::PasteMode;
use crate::input::ScreenBuffer;
use crate::input::encode_key;
use crate::input::encode_mouse;
use crate::input::encode_paste;
use crate::terminal::Terminal;

impl Terminal {
    /// The mouse events the program asked for.
    pub fn mouse_tracking(&self) -> MouseTracking {
        let mode = *self.term.mode();
        if mode.contains(TermMode::MOUSE_MOTION) {
            return MouseTracking::Motion;
        }
        if mode.contains(TermMode::MOUSE_DRAG) {
            return MouseTracking::Drag;
        }
        if mode.contains(TermMode::MOUSE_REPORT_CLICK) {
            return MouseTracking::Click;
        }
        MouseTracking::Off
    }

    /// Which screen buffer shows.
    pub fn screen_buffer(&self) -> ScreenBuffer {
        if self.term.mode().contains(TermMode::ALT_SCREEN) {
            return ScreenBuffer::Alternate;
        }
        ScreenBuffer::Primary
    }

    /// Whether the program wants focus-in and focus-out reports.
    pub fn reports_focus(&self) -> bool {
        self.term.mode().contains(TermMode::FOCUS_IN_OUT)
    }

    /// Whether the cursor is blinking, by the program's choice or the setting.
    pub fn cursor_blinks(&self) -> bool {
        self.term.cursor_style().blinking
    }

    /// The bytes for a mouse event, or `None` when the program did not ask for it.
    pub fn encode_mouse(
        &self,
        event: &MouseEvent,
    ) -> Option<Vec<u8>> {
        encode_mouse(event, self.mouse_tracking(), self.mouse_encoding())
    }

    fn mouse_encoding(&self) -> MouseEncoding {
        let mode = *self.term.mode();
        if mode.contains(TermMode::SGR_MOUSE) {
            return MouseEncoding::Sgr;
        }
        if mode.contains(TermMode::UTF8_MOUSE) {
            return MouseEncoding::Utf8;
        }
        MouseEncoding::Legacy
    }

    /// The bytes for a key press, or `None` for application shortcuts and keys that type nothing.
    pub fn encode_key(
        &self,
        input: &KeyInput,
    ) -> Option<Vec<u8>> {
        encode_key(input, self.cursor_keys())
    }

    fn cursor_keys(&self) -> CursorKeys {
        if self.term.mode().contains(TermMode::APP_CURSOR) {
            return CursorKeys::Application;
        }
        CursorKeys::Normal
    }

    /// The bytes for pasted text.
    pub fn encode_paste(
        &self,
        text: &str,
    ) -> Vec<u8> {
        if self.term.mode().contains(TermMode::BRACKETED_PASTE) {
            return encode_paste(text, PasteMode::Bracketed);
        }
        encode_paste(text, PasteMode::Plain)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::input::Modifiers;
    use crate::input::MouseAction;
    use crate::input::MouseButton;
    use crate::input::ViewPoint;
    use crate::terminal::wire::DaemonFrame;
    use crate::test_support::empty_terminal;

    #[test]
    fn programs_choose_the_input_encodings() {
        let mut terminal = empty_terminal(40, 6);
        let paste = |terminal: &Terminal| terminal.encode_paste("a\nb");
        assert_eq!(paste(&terminal), b"a\rb");
        terminal.apply(&DaemonFrame::Output(
            b"\x1b[?2004h\x1b[?1000h\x1b[?1006h\x1b[?1h".to_vec(),
        ));
        assert_eq!(paste(&terminal), b"\x1b[200~a\rb\x1b[201~");
        let press = MouseEvent {
            button: MouseButton::Left,
            action: MouseAction::Press,
            point: ViewPoint { row: 0, column: 2 },
            modifiers: Modifiers::NONE,
        };
        assert_eq!(
            terminal.encode_mouse(&press),
            Some(b"\x1b[<0;3;1M".to_vec())
        );
        let up = KeyInput {
            key: "up".into(),
            text: None,
            modifiers: Modifiers::NONE,
        };
        assert_eq!(terminal.encode_key(&up), Some(b"\x1bOA".to_vec()));
    }
}
