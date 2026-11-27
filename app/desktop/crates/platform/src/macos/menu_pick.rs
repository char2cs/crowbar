//! The target of every menu item: it remembers which one `AppKit` sent.

use std::cell::Cell;

use objc2::rc::Retained;
use objc2::runtime::AnyObject;
use objc2::{DefinedClass as _, MainThreadMarker, MainThreadOnly, define_class, msg_send, sel};
use objc2_app_kit::NSMenuItem;
use objc2_foundation::NSObject;

define_class!(
    // PLATFORM: macos reports a menu choice only by sending an action to a target object.
    #[unsafe(super(NSObject))]
    #[thread_kind = MainThreadOnly]
    #[ivars = Cell<Option<isize>>]
    #[derive(Debug)]
    pub(crate) struct MenuPick;

    impl MenuPick {
        #[unsafe(method(pick:))]
        fn pick(&self, item: &NSMenuItem) {
            self.ivars().set(Some(item.tag()));
        }
    }
);

impl MenuPick {
    #[expect(unsafe_code, reason = "calls NSObject's designated initializer")]
    pub(crate) fn new(main_thread: MainThreadMarker) -> Retained<Self> {
        let this = Self::alloc(main_thread).set_ivars(Cell::default());
        // SAFETY: `init` is NSObject's designated initializer.
        unsafe { msg_send![super(this), init] }
    }

    #[expect(unsafe_code, reason = "setting a menu item's target and action")]
    pub(crate) fn aim(
        &self,
        item: &NSMenuItem,
    ) {
        let target: &AnyObject = self;
        // SAFETY: the menu's owner keeps this target alive as long as the item.
        unsafe { item.setTarget(Some(target)) };
        // SAFETY: `pick:` takes the sending item, which is all this target declares.
        unsafe { item.setAction(Some(sel!(pick:))) };
    }

    pub(crate) fn tag(&self) -> Option<isize> {
        self.ivars().get()
    }
}

#[cfg(test)]
mod tests {
    use objc2::ClassType as _;

    use super::*;

    #[test]
    fn the_target_answers_the_action_every_item_sends() {
        assert!(MenuPick::class().instance_method(sel!(pick:)).is_some());
    }
}
