use command_palette::{PaletteEntry, PaletteEvent, PalettePanel};
use gpui::{AppContext as _, Context, Window};
use settings_ui::{Chord, Primary};
use shell_keys::ShellCommand;

use super::ShellView;

impl ShellView {
    pub(in crate::shell_view) fn show_palette(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.palette.is_some() {
            return;
        }
        let entries = palette_entries(&self.keymap);
        let panel = cx.new(|cx| PalettePanel::new(entries, window, cx));
        self.subscriptions.push(
            cx.subscribe_in(&panel, window, |this, _, event, window, cx| {
                this.on_palette(event, window, cx);
            }),
        );
        self.palette = Some(panel);
        self.update_overlay(cx);
    }

    fn on_palette(
        &mut self,
        event: &PaletteEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.palette = None;
        self.update_overlay(cx);
        let chosen = match event {
            PaletteEvent::Run(id) => ShellCommand::parse(id),
            _ => None,
        };
        if let Some(command) = chosen {
            self.run_shell_command(command, window, cx);
        }
    }
}

fn palette_entries(keymap: &shell_keys::Keymap) -> Vec<PaletteEntry> {
    ShellCommand::all()
        .filter(|command| *command != ShellCommand::CommandPalette)
        .map(|command| PaletteEntry {
            id: command.id().to_owned(),
            title: command.label().to_owned(),
            shortcut: keymap
                .chord_of(command)
                .and_then(Chord::parse)
                .map(|chord| chord.label(Primary::native())),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_palette_lists_every_command_but_itself_with_its_shortcut() {
        let entries = palette_entries(&shell_keys::Keymap::defaults());
        assert!(
            entries
                .iter()
                .all(|entry| entry.id != ShellCommand::CommandPalette.id())
        );
        let close = entries.iter().find(|entry| entry.id == "tabs.closeActive");
        let expected = ["Ctrl+W", "\u{2318}W"][usize::from(cfg!(target_os = "macos"))];
        assert_eq!(
            close.and_then(|entry| entry.shortcut.as_deref()),
            Some(expected)
        );
    }
}
