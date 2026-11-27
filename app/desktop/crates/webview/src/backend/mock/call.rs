use crate::alert::AlertRequest;
use crate::chooser::FileChoice;
use crate::menu::MenuRequest;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Corners;
use crate::view::Id;
use crate::view::Kind;
use crate::view::ParentWindow;
use crate::view::Visibility;

#[derive(Debug, Clone, PartialEq)]
pub enum Call {
    Create {
        id: Id,
        parent: ParentWindow,
        url: String,
        kind: Kind,
        bounds: Bounds,
        visibility: Visibility,
        background: Option<Color>,
    },
    SetBounds {
        id: Id,
        bounds: Bounds,
    },
    SetVisibility {
        id: Id,
        visibility: Visibility,
    },
    SetCutout {
        id: Id,
        hole: Option<Bounds>,
    },
    SetCorners {
        id: Id,
        corners: Corners,
    },
    SetBackground {
        id: Id,
        color: Color,
    },
    Eval {
        id: Id,
        script: String,
    },
    Navigate {
        id: Id,
        url: String,
    },
    Focus {
        id: Id,
    },
    FocusParent {
        id: Id,
    },
    Snapshot {
        id: Id,
    },
    Settle,
    ClearBrowsingData,
    FollowCaret,
    DrawWidgetsIn {
        scheme: crate::backend::desktop::color_scheme::ColorScheme,
    },
    ChooseFiles {
        parent: ParentWindow,
        choice: FileChoice,
    },
    PopupMenu {
        parent: ParentWindow,
        request: MenuRequest,
    },
    AskAlert {
        parent: ParentWindow,
        request: AlertRequest,
    },
    Destroy {
        id: Id,
    },
}
