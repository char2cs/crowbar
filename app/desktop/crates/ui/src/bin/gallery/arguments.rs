//! Command line of the gallery.

use gpui::App;
use ui::{Theme, ThemeId, ThemeMode};

/// Parsed flags.
pub struct Arguments {
    theme: ThemeId,
    mode: ThemeMode,
    section: Option<String>,
    width: f32,
    height: f32,
}

impl Arguments {
    /// Reads `std::env::args`; unknown flags are ignored.
    pub fn from_env() -> Self {
        let mut parsed = Self { theme: ThemeId::Crowbar, mode: ThemeMode::Light, section: None, width: 1200.0, height: 800.0 };
        let mut args = std::env::args().skip(1);
        while let Some(flag) = args.next() {
            let value = args.next().unwrap_or_default();
            match flag.as_str() {
                "--theme" => parsed.theme = if value == "frosted" { ThemeId::Frosted } else { ThemeId::Crowbar },
                "--mode" => parsed.mode = if value == "dark" { ThemeMode::Dark } else { ThemeMode::Light },
                "--section" => parsed.section = Some(value),
                "--size" => parsed.set_size(&value),
                _ => {}
            }
        }
        parsed
    }

    fn set_size(&mut self, value: &str) {
        let Some((width, height)) = value.split_once('x') else {
            return;
        };
        self.width = width.parse().unwrap_or(self.width);
        self.height = height.parse().unwrap_or(self.height);
    }

    /// Installs the chosen theme.
    pub fn apply(&self, cx: &mut App) {
        Theme::of(cx).with_id(self.theme).with_mode(self.mode).install(cx);
    }

    /// Window width.
    pub fn width(&self) -> f32 {
        self.width
    }

    /// Window height.
    pub fn height(&self) -> f32 {
        self.height
    }

    /// The requested section, if any.
    pub fn section(&self) -> Option<String> {
        self.section.clone()
    }
}
