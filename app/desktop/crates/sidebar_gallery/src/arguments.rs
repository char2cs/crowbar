//! Command line of the sidebar gallery.

use gpui::App;
use sidebar::{DemoData, DropMode, Mode, Preferences, RowId, Visibility};
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
    drag: Option<String>,
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
            drag: flag("drag"),
        }
    }

    pub(crate) fn apply(
        &self,
        cx: &mut App,
    ) {
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

    pub(crate) fn drag(&self) -> Option<(RowId, RowId, DropMode)> {
        let mut parts = self.drag.as_deref()?.split(':');
        let (subject, target) = (parts.next()?, parts.next()?);
        let mode = match parts.next()? {
            "before" => DropMode::Before,
            "after" => DropMode::After,
            _ => DropMode::Into,
        };
        Some((RowId::from(subject), RowId::from(target), mode))
    }

    fn visibility(
        &self,
        default: Visibility,
    ) -> Visibility {
        if self.closed {
            return Visibility::Closed;
        }
        default
    }

    pub(crate) fn preferences(&self) -> Preferences {
        let default = Preferences::default();
        Preferences {
            mode: self.panel,
            width: self.width.unwrap_or(default.width),
            visibility: self.visibility(default.visibility),
            ..default
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn with_drag(drag: Option<&str>) -> Arguments {
        Arguments {
            theme: ThemeId::Crowbar,
            mode: ThemeMode::Dark,
            size: DEFAULT_SIZE,
            data: String::new(),
            panel: Mode::Chats,
            width: None,
            closed: false,
            drag: drag.map(str::to_owned),
        }
    }

    #[test]
    fn sizes_and_drags_parse_from_their_flags() {
        assert_eq!(parse_size("800x600"), Some((800.0, 600.0)));
        assert_eq!(parse_size("800"), None);
        let want = Some((RowId::from("a"), RowId::from("b"), DropMode::After));
        assert_eq!(with_drag(Some("a:b:after")).drag(), want);
        assert_eq!(with_drag(Some("a")).drag(), None);
        assert_eq!(with_drag(None).drag(), None);
    }

    #[test]
    fn a_closed_gallery_starts_with_the_sidebar_closed() {
        let mut arguments = with_drag(None);
        arguments.closed = true;
        assert_eq!(arguments.preferences().visibility, Visibility::Closed);
    }
}
