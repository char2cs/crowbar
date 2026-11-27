use std::path::PathBuf;
use std::sync::Arc;

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum PageEvent {
    Navigating(String),
    Loading(String),
    Loaded(String),
    Title(String),
    FilesDropped { paths: Vec<PathBuf>, x: f64, y: f64 },
    Pressed,
    PointerMoved { x: f64, y: f64 },
}

pub type PageHandler = Arc<dyn Fn(PageEvent) -> bool + Send + Sync + 'static>;
