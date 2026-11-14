use gpui::App;
use ui::{Theme, ThemeId, ThemeMode};

const DEFAULT_SIZE: (f32, f32) = (1200.0, 800.0);

pub(crate) struct Arguments {
    mode: ThemeMode,
    scenario: String,
    size: (f32, f32),
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
        let mode = match flag("mode").as_deref() {
            Some("dark") => ThemeMode::Dark,
            _ => ThemeMode::Light,
        };
        Self {
            mode,
            scenario: flag("scenario").unwrap_or_else(|| "split".into()),
            size: flag("size")
                .as_deref()
                .and_then(parse_size)
                .unwrap_or(DEFAULT_SIZE),
        }
    }

    pub(crate) fn apply(&self, cx: &mut App) {
        Theme::of(cx)
            .with_id(ThemeId::Crowbar)
            .with_mode(self.mode)
            .install(cx);
    }

    pub(crate) fn size(&self) -> (f32, f32) {
        self.size
    }

    pub(crate) fn scenario(&self) -> String {
        self.scenario.clone()
    }
}
