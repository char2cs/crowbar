mod error;
mod find;
mod hyperlink;
mod input;
mod snapshot;
mod terminal;
#[cfg(test)]
mod test_support;

pub use error::Error;
pub use find::{SearchOptions, SearchQuery, SearchSummary};
pub use hyperlink::{Link, LinkTarget, resolve_file_path};
pub use input::{
    CellHit, CellMetrics, CellSide, ClickTracker, DragEdge, KeyInput, Modifiers, MouseAction,
    MouseButton, MouseEvent, MouseTracking, ScreenBuffer, ScrollAccumulator, ScrollbarThumb,
    SelectionKind, ViewPoint, WheelRoute, cells_f32, floor_cells, format_dropped_paths,
    route_wheel,
};
pub use snapshot::{
    BackgroundSpan, CursorCell, CursorShape, Highlight, Palette, Rgb, Row, Segment, Snapshot,
    StyleFlags, StyleSpan, Underline,
};
pub use terminal::{ClientMessage, Config, DaemonFrame, Event, Terminal, sanitize_title};
