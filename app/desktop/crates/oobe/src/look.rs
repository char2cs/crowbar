use gpui::{Hsla, Pixels, hsla, px, rgb};

pub(crate) const TAGLINE: &str = "The IDE where agents do the heavy lifting.";
pub(crate) const WELCOME_LABEL: &str = "Get started with agents";
pub(crate) const HOST_LABEL: &str = "Let\u{2019}s build together";
pub(crate) const CHECK_TITLE: &str = "System check";
pub(crate) const CHECK_CAPTION: &str = "Making sure everything's in place before you start.";
pub(crate) const GIT_BLOCKED: &str = "Git is required. Install it from git-scm.com.";
pub(crate) const SPACE_TITLE: &str = "Add your first space";
pub(crate) const SPACE_ABOUT: &str = "A space is the place where your repositories live. They don't have to be inside the space's folder \u{2014} but it's a natural home for them.";
pub(crate) const SPACE_MEMORY: &str = "Spaces are also where cross-repo plans, chats, and context get saved. Think of it as the shared memory for everything you're building.";
pub(crate) const CHOOSE_LABEL: &str = "Choose a folder";
pub(crate) const RETRY_LABEL: &str = "Check again";
pub(crate) const CONTINUE_LABEL: &str = "Continue";

pub(crate) const CARD_WIDTH: Pixels = px(448.0);
pub(crate) const CARD_PADDING: Pixels = px(40.0);
pub(crate) const CARD_RADIUS: Pixels = px(24.0);
pub(crate) const CARD_LOGO: Pixels = px(180.0);
pub(crate) const WELCOME_LOGO: Pixels = px(360.0);
pub(crate) const HOST_LOGO: Pixels = px(176.0);
pub(crate) const STAGE_GAP: Pixels = px(32.0);
pub(crate) const BLOCK_GAP: Pixels = px(8.0);
pub(crate) const ROW_PAD: Pixels = px(12.0);
pub(crate) const ROW_GAP: Pixels = px(16.0);
pub(crate) const HEADING: Pixels = px(24.0);
pub(crate) const BODY: Pixels = px(14.0);
pub(crate) const CAPTION: Pixels = px(12.0);
pub(crate) const TAGLINE_SIZE: Pixels = px(16.0);
pub(crate) const TAGLINE_WIDTH: Pixels = px(320.0);
pub(crate) const STATUS_ICON: Pixels = px(20.0);
pub(crate) const PILL_PAD_X: Pixels = px(32.0);
pub(crate) const PILL_PAD_Y: Pixels = px(10.0);
pub(crate) const DISABLED_OPACITY: f32 = 0.4;

pub(crate) const BASE: u32 = 0x008d_a0ce;
pub(crate) const TEAL: u32 = 0x0073_bfc4;
pub(crate) const ORANGE: u32 = 0x00ff_810a;
pub(crate) const BRAND_GROUND: u32 = 0x0098_a86a;
pub(crate) const GRADIENT_ANGLE: f32 = 135.0;
pub(crate) const ORANGE_STRENGTH: f32 = 0.85;

pub(crate) fn white(alpha: f32) -> Hsla {
    hsla(0.0, 0.0, 1.0, alpha)
}

pub(crate) fn black(alpha: f32) -> Hsla {
    hsla(0.0, 0.0, 0.0, alpha)
}

pub(crate) fn color(value: u32) -> Hsla {
    rgb(value).into()
}

pub(crate) const FAINT: f32 = 0.5;
pub(crate) const SOFT: f32 = 0.6;
pub(crate) const GLASS: f32 = 0.1;
pub(crate) const RIM: f32 = 0.2;
pub(crate) const TAGLINE_INK: f32 = 0.8;
pub(crate) const RULE: f32 = 0.1;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_helpers_make_ink_and_hues_of_the_given_strength() {
        assert!((white(0.5).a - 0.5).abs() < f32::EPSILON);
        assert!((white(0.5).l - 1.0).abs() < f32::EPSILON);
        assert!(black(1.0).l.abs() < f32::EPSILON);
        let orange: Hsla = rgb(ORANGE).into();
        assert_eq!(color(ORANGE), orange);
    }
}
