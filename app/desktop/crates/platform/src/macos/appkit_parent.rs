use std::ffi::c_void;
use std::ptr::NonNull;

use objc2::MainThreadMarker;
use objc2::rc::Retained;
use objc2_app_kit::{NSApplication, NSView};
use raw_window_handle::{
    AppKitWindowHandle, HandleError, HasWindowHandle, RawWindowHandle, WindowHandle,
};

/// An `NSView` of one of the app's open windows that a native web view can be embedded into,
/// presented as an `AppKit` window handle.
///
/// It holds a strong reference to the view, so the view outlives every handle borrowed from it.
#[derive(Debug, Clone)]
pub struct AppkitParent(Retained<NSView>);

impl AppkitParent {
    /// Finds the view at `address` among the views of the app's open windows.
    ///
    /// Returns `None` when no open window shows a view at that address, so a stale or made-up
    /// address never becomes a parent.
    pub fn find(
        address: usize,
        main_thread: MainThreadMarker,
    ) -> Option<Self> {
        NSApplication::sharedApplication(main_thread)
            .windows()
            .to_vec()
            .into_iter()
            .filter_map(|window| window.contentView())
            .find_map(|content| view_at(content, address))
            .map(Self)
    }
}

fn view_at(
    view: Retained<NSView>,
    address: usize,
) -> Option<Retained<NSView>> {
    first_in_tree(
        view,
        &|view: &Retained<NSView>| Retained::as_ptr(view).addr() == address,
        &|view: &Retained<NSView>| view.subviews().to_vec(),
    )
}

fn first_in_tree<T>(
    node: T,
    matches: &impl Fn(&T) -> bool,
    children: &impl Fn(&T) -> Vec<T>,
) -> Option<T> {
    if matches(&node) {
        return Some(node);
    }
    children(&node)
        .into_iter()
        .find_map(|child| first_in_tree(child, matches, children))
}

impl HasWindowHandle for AppkitParent {
    #[expect(
        unsafe_code,
        reason = "WindowHandle::borrow_raw is unsafe; the parent holds the view alive"
    )]
    fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
        let view = NonNull::from(&*self.0).cast::<c_void>();
        let handle = RawWindowHandle::AppKit(AppKitWindowHandle::new(view));
        // SAFETY: `self` holds a strong reference to the view for as long as the handle borrows it.
        Ok(unsafe { WindowHandle::borrow_raw(handle) })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tree() -> impl Fn(&u8) -> Vec<u8> {
        |id| match id {
            1 => vec![2, 3],
            3 => vec![4],
            _ => Vec::new(),
        }
    }

    #[test]
    fn finds_the_first_matching_node_each_parent_before_its_children() {
        let cases = [
            ("a grandchild below the root", 4..=4, Some(4)),
            ("a node in no branch", 9..=9, None),
            ("a parent before its child", 3..=4, Some(3)),
        ];
        for (what, ids, found) in cases {
            let matches = |id: &u8| ids.contains(id);
            assert_eq!(first_in_tree(1, &matches, &tree()), found, "{what}");
        }
    }
}
