use std::sync::Arc;

use crate::view::bounds::Bounds;
use crate::view::color::Color;
use crate::view::kind::Kind;
use crate::view::page_event::PageHandler;
use crate::view::visibility::Visibility;

pub type IpcHandler = Arc<dyn Fn(String) + Send + Sync + 'static>;

#[derive(Clone)]
pub struct Config {
    pub url: String,
    pub bounds: Bounds,
    pub kind: Kind,
    pub visibility: Visibility,
    pub background: Option<Color>,
    pub initialization_script: Option<String>,
    pub on_ipc: IpcHandler,
    pub on_page: Option<PageHandler>,
}

impl std::fmt::Debug for Config {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Config")
            .field("url", &self.url)
            .field("bounds", &self.bounds)
            .field("kind", &self.kind)
            .field("visibility", &self.visibility)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rect(
        x: f64,
        y: f64,
        width: f64,
        height: f64,
    ) -> Bounds {
        Bounds {
            x,
            y,
            width,
            height,
        }
    }

    #[test]
    fn debug_shows_the_page_and_hides_the_script_and_handler() {
        let config = Config {
            url: "crowbar://local/x".into(),
            bounds: rect(0.0, 0.0, 1.0, 1.0),
            kind: Kind::Core,
            visibility: Visibility::Hidden,
            background: None,
            initialization_script: Some("secret-script".into()),
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        };
        let text = format!("{config:?}");
        assert!(text.contains("crowbar://local/x"), "url shown: {text}");
        assert!(!text.contains("secret-script"), "script hidden: {text}");
    }
}
