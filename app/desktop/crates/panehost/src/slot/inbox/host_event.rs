use browser::PageUpdate;
use protocol::{
    CommandId, DisplayText, ExternalUrl, FsPath, PaneId, PaneOpen, PaneState, PaneTitle, PaneToast,
    Params, SettingKey, ViewedFile,
};
use serde_json::Value;

#[derive(Debug, Clone, PartialEq)]
pub enum HostEvent {
    TitleChanged {
        pane: PaneId,
        title: PaneTitle,
    },
    ParamsChanged {
        pane: PaneId,
        params: Params,
    },
    StateChanged {
        pane: PaneId,
        state: PaneState,
    },
    DirtyChanged {
        pane: PaneId,
        dirty: bool,
    },
    StateDiscarded {
        pane: PaneId,
    },
    OpenRequested {
        from: PaneId,
        open: PaneOpen,
    },
    CommandRequested {
        pane: PaneId,
        command: CommandId,
    },
    OpenExternal {
        url: ExternalUrl,
    },
    Reveal {
        path: FsPath,
    },
    Mounted {
        pane: PaneId,
    },
    ReviewViewedSet {
        pane: PaneId,
        files: Vec<ViewedFile>,
        viewed: bool,
    },
    DocumentEdited {
        pane: PaneId,
        text: String,
    },
    SourceRequested {
        pane: PaneId,
    },
    DocumentSaveRequested {
        pane: PaneId,
    },
    SettingRequested {
        pane: PaneId,
        key: SettingKey,
        value: Value,
    },
    HoverChanged {
        pane: PaneId,
        hovered: bool,
    },
    PointerMoved {
        pane: PaneId,
        x: f64,
        y: f64,
    },
    PageFocused {
        pane: PaneId,
    },
    ToastShown {
        pane: PaneId,
        toast: PaneToast,
    },
    ToastClosed {
        pane: PaneId,
        key: DisplayText,
    },
    OpenRefused {
        pane: PaneId,
        title: Option<DisplayText>,
        message: DisplayText,
    },
    BrowserPage {
        pane: PaneId,
        update: PageUpdate,
    },
}
