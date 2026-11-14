//! Command line of the sidebar gallery.

use gpui::App;
use sidebar::{DemoData, Mode, Preferences, Visibility};
use ui::{Theme, ThemeId, ThemeMode};

const DEFAULT_SIZE: (f32, f32) = (1200.0, 800.0);

pub(crate) struct Arguments {
    theme: ThemeId,
    mode: ThemeMode,
    size: (f32, f32),
    data: String,
    panel: Mode,
    width: Option<f32>,
    closed: bool,
}

fn flag(name: &str) -> Option<String> {
    let args: Vec<String> = std::env::args().collect();
    let position = args
        .iter()
        .position(|arg| arg.strip_prefix("--") == Some(name))?;
    args.get(position + 1).cloned()
}

fn parse_size(value: &str) -> Option<(f32, f32)> {
    let (width, height) = value.split_once('x')?;
    Some((width.parse().ok()?, height.parse().ok()?))
}

impl Arguments {
    pub(crate) fn from_env() -> Self {
        Self {
            theme: match flag("theme").as_deref() {
                Some("frosted") => ThemeId::Frosted,
                _ => ThemeId::Crowbar,
            },
            mode: match flag("mode").as_deref() {
                Some("light") => ThemeMode::Light,
                _ => ThemeMode::Dark,
            },
            size: flag("size")
                .as_deref()
                .and_then(parse_size)
                .unwrap_or(DEFAULT_SIZE),
            data: flag("data").unwrap_or_default(),
            panel: match flag("panel").as_deref() {
                Some("files") => Mode::Files,
                Some("changes") => Mode::Changes,
                _ => Mode::Chats,
            },
            width: flag("width").and_then(|value| value.parse().ok()),
            closed: std::env::args().any(|arg| arg == "--closed"),
        }
    }

    pub(crate) fn apply(&self, cx: &mut App) {
        Theme::of(cx)
            .with_id(self.theme)
            .with_mode(self.mode)
            .install(cx);
    }

    pub(crate) fn size(&self) -> (f32, f32) {
        self.size
    }

    pub(crate) fn data(&self) -> DemoData {
        match self.data.as_str() {
            "rich" => DemoData::rich(),
            "loading" => DemoData::loading(),
            _ => DemoData::reference(),
        }
    }

    pub(crate) fn preferences(&self) -> Preferences {
        let default = Preferences::default();
        Preferences {
            mode: self.panel,
            width: self.width.unwrap_or(default.width),
            visibility: if self.closed {
                Visibility::Closed
            } else {
                default.visibility
            },
            ..default
        }
    }
}
