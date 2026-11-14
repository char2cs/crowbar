//! Command line of the preview.

use gpui::App;
use settings_ui::PageId;
use ui::{Theme, ThemeId, ThemeMode};

const DEFAULT_SIZE: (f32, f32) = (1200.0, 800.0);

pub(crate) struct Arguments {
    theme: ThemeId,
    mode: ThemeMode,
    page: Option<String>,
    query: Option<String>,
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
        let theme = match flag("theme").as_deref() {
            Some("frosted") => ThemeId::Frosted,
            _ => ThemeId::Crowbar,
        };
        let mode = match flag("mode").as_deref() {
            Some("dark") => ThemeMode::Dark,
            _ => ThemeMode::Light,
        };
        Self {
            theme,
            mode,
            page: flag("page"),
            query: flag("query"),
            size: flag("size")
                .as_deref()
                .and_then(parse_size)
                .unwrap_or(DEFAULT_SIZE),
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

    pub(crate) fn page(&self) -> Option<PageId> {
        self.page.as_deref().map(PageId::from)
    }

    pub(crate) fn query(&self) -> Option<&str> {
        self.query.as_deref()
    }
}
