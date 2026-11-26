use settings::{Key, Value, Write as _};

use crate::shell::inbox::logged::emit;
use crate::shell::shell_view::ShellView;

mod zoom_event;
use zoom_event::ZoomEvent;

const FONT_SIZE: &str = "terminalFontSize";

impl ShellView {
    pub(crate) fn store_terminal_font_size(
        &mut self,
        size: f32,
    ) {
        let Some(backend) = self.settings_backend() else {
            return emit(&ZoomEvent::SettingsClosed);
        };
        if let Err(error) = backend.set(&Key::new(FONT_SIZE), Value::Number(f64::from(size))) {
            emit(&ZoomEvent::Unkept { error: &error });
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, Read as _, Value};
    use terminalview::Event;

    use crate::shell::shell_view::test_support::{tell, with_terminal};

    #[gpui::test]
    fn a_terminal_zoom_is_stored_as_the_terminal_font_size_setting(cx: &mut TestAppContext) {
        let (mut opened, terminal) = with_terminal(cx);
        tell(&mut opened, (&terminal, Event::FontSizeChanged(18.0)));
        let stored = opened.shell.update(&mut opened.window, |shell, _| {
            shell
                .settings_backend()
                .and_then(|backend| backend.get(&Key::new("terminalFontSize")))
        });
        assert_eq!(stored, Some(Value::Number(18.0)));
    }
}
