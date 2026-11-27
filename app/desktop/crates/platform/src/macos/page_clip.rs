use std::cell::Cell;
use std::ffi::c_void;
use std::ptr::NonNull;

use objc2::rc::Retained;
use objc2::{DefinedClass as _, MainThreadOnly, define_class, msg_send};
use objc2_app_kit::NSView;
use objc2_core_foundation::{CGPoint, CGRect, CGSize};
use objc2_quartz_core::{CAShapeLayer, kCAFillRuleEvenOdd};
use raw_window_handle::{
    AppKitWindowHandle, HandleError, HasWindowHandle, RawWindowHandle, WindowHandle,
};

use crate::error::Error;
use crate::macos::appkit_parent::AppkitParent;
use crate::macos::clip_shape::{CORNERS, ClipShape};

define_class!(
    // PLATFORM: macos a child view paints and takes the pointer as a rectangle unless its holder says otherwise.
    #[unsafe(super(NSView))]
    #[thread_kind = MainThreadOnly]
    #[ivars = Cell<ClipShape>]
    #[derive(Debug)]
    struct ClipView;

    impl ClipView {
        #[unsafe(method(isFlipped))]
        fn flipped(&self) -> bool {
            true
        }

        #[unsafe(method_id(hitTest:))]
        fn hit(&self, point: CGPoint) -> Option<Retained<NSView>> {
            self.taking(point)
        }
    }
);

impl ClipView {
    #[expect(unsafe_code, reason = "calls NSView's own hit test")]
    fn taking(
        &self,
        point: CGPoint,
    ) -> Option<Retained<NSView>> {
        // SAFETY: the superview is read on the main thread, which owns the tree.
        let above = unsafe { self.superview() };
        let local = self.convertPoint_fromView(point, above.as_deref());
        if self.ivars().get().cuts(local) {
            return None;
        }
        // SAFETY: `hitTest:` of the superclass takes a point and returns a view or nil.
        let hit: Option<Retained<NSView>> = unsafe { msg_send![super(self), hitTest: point] };
        let own: &NSView = self;
        hit.filter(|view| !std::ptr::eq(Retained::as_ptr(view), std::ptr::from_ref(own)))
    }
}

#[derive(Debug)]
pub struct PageClip(Retained<ClipView>);

impl PageClip {
    #[expect(unsafe_code, reason = "calls NSView's designated initializer")]
    pub fn within(parent: &AppkitParent) -> Result<Self, Error> {
        let main_thread = objc2::MainThreadMarker::new().ok_or(Error::Clip)?;
        let this = ClipView::alloc(main_thread).set_ivars(Cell::default());
        // SAFETY: `initWithFrame:` is NSView's designated initializer and takes a rectangle.
        let view: Retained<ClipView> =
            unsafe { msg_send![super(this), initWithFrame: CGRect::ZERO] };
        view.setWantsLayer(true);
        parent.view().addSubview(&view);
        Ok(Self(view))
    }

    #[expect(unsafe_code, reason = "reads the superview")]
    pub fn set_frame(
        &self,
        frame: [f64; CORNERS],
    ) {
        let [x, y, width, height] = frame;
        // SAFETY: the superview is read on the main thread, which owns the tree.
        let Some(parent) = (unsafe { self.0.superview() }) else {
            return;
        };
        let mut frame = CGRect::new(CGPoint::new(x, y), CGSize::new(width, height));
        if !parent.isFlipped() {
            frame.origin.y = from_bottom(parent.bounds().size.height, frame);
        }
        if self.0.frame() != frame {
            self.0.setFrame(frame);
            self.mask();
        }
    }

    pub fn set_shape(
        &self,
        shape: ClipShape,
    ) {
        if self.0.ivars().replace(shape) != shape {
            self.mask();
        }
    }

    #[expect(unsafe_code, reason = "reads the superview")]
    pub fn hand_keys_to_parent(&self) {
        // SAFETY: the superview is read on the main thread, which owns the tree.
        let parent = unsafe { self.0.superview() };
        if let Some(window) = self.0.window() {
            window.makeFirstResponder(parent.as_deref().map(|view| &**view));
        }
    }

    #[expect(
        unsafe_code,
        reason = "sets a layer mask and reads QuartzCore's fill rule"
    )]
    fn mask(&self) {
        let Some(layer) = self.0.layer() else {
            return;
        };
        let shape = self.0.ivars().get();
        // PLATFORM: macos a mask costs an offscreen pass, so a plain rectangle wears none.
        let mask = (shape != ClipShape::default()).then(|| {
            let size = self.0.frame().size;
            let mask = CAShapeLayer::new();
            // SAFETY: the fill rule is a constant string QuartzCore owns.
            mask.setFillRule(unsafe { kCAFillRuleEvenOdd });
            mask.setPath(Some(&shape.outline(size)));
            mask
        });
        // SAFETY: the mask is a new layer with no superlayer.
        unsafe { layer.setMask(mask.as_deref().map(|mask| &**mask)) };
    }
}

fn from_bottom(
    height: f64,
    frame: CGRect,
) -> f64 {
    height - frame.origin.y - frame.size.height
}

impl Drop for PageClip {
    fn drop(&mut self) {
        self.0.removeFromSuperview();
    }
}

impl HasWindowHandle for PageClip {
    #[expect(
        unsafe_code,
        reason = "WindowHandle::borrow_raw is unsafe; the clip holds the view alive"
    )]
    fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
        let view = NonNull::from(&**self.0).cast::<c_void>();
        let handle = RawWindowHandle::AppKit(AppKitWindowHandle::new(view));
        // SAFETY: `self` holds a strong reference to the view for as long as the handle borrows it.
        Ok(unsafe { WindowHandle::borrow_raw(handle) })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_frame_is_counted_from_the_bottom_of_a_parent_that_is_not_flipped() {
        let frame = CGRect::new(CGPoint::new(0.0, 10.0), CGSize::new(5.0, 20.0));
        let y = from_bottom(100.0, frame);
        assert!((y - 70.0).abs() < f64::EPSILON, "{y}");
    }
}
