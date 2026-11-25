use crate::alert::AlertRequest;
use crate::chooser::FileChoice;
use crate::menu::MenuRequest;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Id;
use crate::view::Kind;
use crate::view::ParentWindow;
use crate::view::Visibility;

/// One operation a [`MockPlatform`](crate::MockPlatform) recorded.
#[derive(Debug, Clone, PartialEq)]
pub enum Call {
    /// A view was created.
    Create {
        /// The new view.
        id: Id,
        /// Its parent window.
        parent: ParentWindow,
        /// The page it loads.
        url: String,
        /// Core or extension view.
        kind: Kind,
        /// Initial placement.
        bounds: Bounds,
        /// Initial visibility.
        visibility: Visibility,
        /// Colour shown before the page paints.
        background: Option<Color>,
    },
    /// A view was moved or resized.
    SetBounds {
        /// The view.
        id: Id,
        /// The new placement.
        bounds: Bounds,
    },
    /// A view was shown or hidden.
    SetVisibility {
        /// The view.
        id: Id,
        /// The new visibility.
        visibility: Visibility,
    },
    /// A view got another colour to show before its page paints.
    SetBackground {
        /// The view.
        id: Id,
        /// The new colour.
        color: Color,
    },
    /// A script was run.
    Eval {
        /// The view.
        id: Id,
        /// The script text.
        script: String,
    },
    /// A view loaded another page.
    Navigate {
        /// The view.
        id: Id,
        /// The new page.
        url: String,
    },
    /// A view received keyboard focus.
    Focus {
        /// The view.
        id: Id,
    },
    /// Keyboard focus went back to the parent window.
    FocusParent {
        /// The view that had focus.
        id: Id,
    },
    /// The pixels of a view were asked for.
    Snapshot {
        /// The view.
        id: Id,
    },
    /// Every earlier request was waited for.
    Settle,
    /// The browser views' storage was cleared.
    ClearBrowsingData,
    /// The desktop's caret blink was followed.
    FollowCaret,
    /// The desktop's own widgets were asked to draw in another colour scheme.
    DrawWidgetsIn {
        /// The scheme they draw in now.
        scheme: crate::backend::desktop::color_scheme::ColorScheme,
    },
    /// The desktop's file chooser was opened.
    ChooseFiles {
        /// The window it was opened over.
        parent: ParentWindow,
        /// What it asked.
        choice: FileChoice,
    },
    /// The desktop's context menu was popped up.
    PopupMenu {
        /// The window it was popped up over.
        parent: ParentWindow,
        /// What it showed and where.
        request: MenuRequest,
    },
    /// The desktop's message dialog was asked.
    AskAlert {
        /// The window it was asked over.
        parent: ParentWindow,
        /// What it asked.
        request: AlertRequest,
    },
    /// A view was destroyed.
    Destroy {
        /// The view.
        id: Id,
    },
}
