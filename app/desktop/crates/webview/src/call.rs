use crate::bounds::Bounds;
use crate::parent_window::ParentWindow;
use crate::visibility::Visibility;
use crate::web_view_id::WebViewId;
use crate::web_view_kind::WebViewKind;

/// One operation a [`MockPlatform`](crate::MockPlatform) recorded.
#[derive(Debug, Clone, PartialEq)]
pub enum Call {
    /// A view was created.
    Create {
        /// The new view.
        id: WebViewId,
        /// Its parent window.
        parent: ParentWindow,
        /// The page it loads.
        url: String,
        /// Core or extension view.
        kind: WebViewKind,
        /// Initial placement.
        bounds: Bounds,
        /// Initial visibility.
        visibility: Visibility,
    },
    /// A view was moved or resized.
    SetBounds {
        /// The view.
        id: WebViewId,
        /// The new placement.
        bounds: Bounds,
    },
    /// A view was shown or hidden.
    SetVisibility {
        /// The view.
        id: WebViewId,
        /// The new visibility.
        visibility: Visibility,
    },
    /// A script was run.
    Eval {
        /// The view.
        id: WebViewId,
        /// The script text.
        script: String,
    },
    /// A view loaded another page.
    Navigate {
        /// The view.
        id: WebViewId,
        /// The new page.
        url: String,
    },
    /// A view received keyboard focus.
    Focus {
        /// The view.
        id: WebViewId,
    },
    /// Keyboard focus went back to the parent window.
    FocusParent {
        /// The view that had focus.
        id: WebViewId,
    },
    /// A view was destroyed.
    Destroy {
        /// The view.
        id: WebViewId,
    },
}
