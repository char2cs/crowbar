//! The window beneath a modal layer, captured once for the top layer to paint under its scrim.

use std::sync::Arc;

use gpui::{
    AnyElement, App, Bounds, Context, DevicePixels, ElementId, Entity, IntoElement, ObjectFit,
    ParentElement, Pixels, RenderImage, Size, Styled, StyledImage, Window, div, img,
};
use image::{Frame, RgbaImage};

use crate::foundation::opaque_debug;

mod blur;

/// What a modal layer shows of the window beneath it. The top layer is a window of its own: a
/// compositor blends its translucent scrim over the window but cannot blur through it, and
/// without one only its opaque pixels show. So the layer paints a capture of the window itself,
/// taken again whenever the window's content changes.
#[derive(Default)]
pub struct Backdrop {
    taken: Option<(Size<Pixels>, u64)>,
    plain: Option<Arc<RenderImage>>,
    blurred: Option<Arc<RenderImage>>,
}

impl Backdrop {
    /// The backdrop of the layer `id`, kept while that layer renders.
    pub fn of(
        id: impl Into<ElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> Entity<Self> {
        window.use_keyed_state(id, cx, |_, cx: &mut Context<'_, Self>| {
            cx.on_release(Self::release).detach();
            Self::default()
        })
    }

    /// The window as it is, for a scrim that only dims; `None` where a compositor blends the
    /// scrim over the live window instead.
    pub fn plain(
        &mut self,
        window: &mut Window,
    ) -> Option<AnyElement> {
        if window.is_composited() {
            return None;
        }
        let bounds = self.retake(window)?;
        let image = self.plain.clone()?;
        Some(Self::picture(image, bounds))
    }

    /// The window blurred by `radius` (CSS `blur()`), with or without a compositor; the first
    /// call after a capture blurs it.
    pub fn blurred(
        &mut self,
        radius: Pixels,
        window: &mut Window,
    ) -> Option<AnyElement> {
        let bounds = self.retake(window)?;
        if self.blurred.is_none() {
            let device = DevicePixels::from(radius.scale(window.scale_factor()));
            let sigma = usize::try_from(device.0).ok()?;
            let captured = rgba(self.plain.as_deref()?)?;
            self.blurred = Some(render_image(blur::blurred(&captured, sigma)));
        }
        let image = self.blurred.clone()?;
        Some(Self::picture(image, bounds))
    }

    fn retake(
        &mut self,
        window: &mut Window,
    ) -> Option<Bounds<Pixels>> {
        let bounds = window.owner_bounds()?;
        let now = (bounds.size, window.owner_version());
        if self.taken == Some(now) {
            return Some(bounds);
        }
        self.drop_images(window);
        let captured = window.capture_owner()?;
        self.plain = Some(render_image(captured));
        self.taken = Some(now);
        Some(bounds)
    }

    fn drop_images(
        &mut self,
        window: &mut Window,
    ) {
        for image in [self.plain.take(), self.blurred.take()]
            .into_iter()
            .flatten()
        {
            log_failure(window.drop_image(image));
        }
    }

    fn release(
        &mut self,
        cx: &mut App,
    ) {
        for image in [self.plain.take(), self.blurred.take()]
            .into_iter()
            .flatten()
        {
            cx.drop_image(image, None);
        }
    }

    fn picture(
        image: Arc<RenderImage>,
        bounds: Bounds<Pixels>,
    ) -> AnyElement {
        div()
            .absolute()
            .left(bounds.origin.x)
            .top(bounds.origin.y)
            .w(bounds.size.width)
            .h(bounds.size.height)
            .child(img(image).size_full().object_fit(ObjectFit::Fill))
            .into_any_element()
    }
}

fn log_failure<E: std::fmt::Display>(result: Result<(), E>) {
    if let Err(error) = result {
        tracing::warn!(%error, "dropping a backdrop capture failed");
    }
}

fn render_image(bgra: RgbaImage) -> Arc<RenderImage> {
    Arc::new(RenderImage::new(vec![Frame::new(bgra)]))
}

fn rgba(image: &RenderImage) -> Option<RgbaImage> {
    let size = image.size(0);
    let width = u32::try_from(size.width.0).ok()?;
    let height = u32::try_from(size.height.0).ok()?;
    RgbaImage::from_raw(width, height, image.as_bytes(0)?.to_vec())
}

opaque_debug!(Backdrop);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::floating::Portal;
    use crate::test_support::probe::open_floating;

    fn plain(
        window: &mut Window,
        cx: &mut App,
    ) -> gpui::Div {
        let backdrop = Backdrop::of("b", window, cx);
        div().children(backdrop.update(cx, |backdrop, _| backdrop.plain(window)))
    }

    #[gpui::test]
    fn without_a_compositor_the_plain_backdrop_shows_the_whole_window(cx: &mut TestAppContext) {
        let (mut owner, mut top) = open_floating(cx, |_, _| Portal::window("p", plain));
        owner.update(|window, _| window.refresh());
        owner.run_until_parked();
        let whole = top
            .update(|window, _| window.owner_bounds())
            .unwrap_or_default();
        let content = top.update(|window, _| window.content_region().to_vec());
        assert!(
            content.contains(&whole),
            "the whole window shows: {content:?}"
        );
    }
}
