use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
};
use ui::{ProgressBar, Surface, Theme, UiText};

use crate::machine::MigrationStep;

const WIDTH: f32 = 360.0;
const CAPTION_GAP: f32 = 12.0;
const FOOTNOTE_MARGIN: f32 = 10.0;
const STACK_GAP: f32 = 14.0;

#[derive(Debug, IntoElement)]
pub(crate) struct WaitingPanel {
    headline: &'static str,
    caption: SharedString,
    count: Option<SharedString>,
    fraction: Option<f32>,
    previous: f32,
    footnote: Option<&'static str>,
}

impl WaitingPanel {
    pub(crate) fn starting() -> Self {
        Self {
            headline: "Starting Crowbar",
            caption: "Waiting for the background service".into(),
            count: None,
            fraction: None,
            previous: 0.0,
            footnote: None,
        }
    }

    pub(crate) fn migrating(
        step: Option<&MigrationStep>,
        fraction: Option<f32>,
        previous: f32,
    ) -> Self {
        Self {
            headline: "Upgrading your data",
            caption: caption_of(step),
            count: step.map(|step| format!("{} / {}", step.index(), step.total()).into()),
            fraction,
            previous,
            footnote: Some("Crowbar opens when this finishes. Please keep it open."),
        }
    }
}

fn caption_of(step: Option<&MigrationStep>) -> SharedString {
    let Some(step) = step else {
        return "Preparing the upgrade".into();
    };
    step.name().to_owned().into()
}

impl WaitingPanel {
    fn caption_row(
        &self,
        theme: &Theme,
    ) -> impl IntoElement {
        let muted = theme.palette(Surface::Window).muted_foreground();
        let count = self.count.clone().map(|count| {
            div()
                .flex_none()
                .font_family(theme.mono_family())
                .text_size(UiText::Xs.size(theme))
                .child(count)
        });
        div()
            .flex()
            .justify_between()
            .gap(px(CAPTION_GAP))
            .w(px(WIDTH))
            .text_size(UiText::Sm.size(theme))
            .text_color(muted)
            .child(div().min_w_0().truncate().child(self.caption.clone()))
            .children(count)
    }
}

impl RenderOnce for WaitingPanel {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let muted = theme.palette(Surface::Window).muted_foreground();
        let bar = ProgressBar::new("startup-progress", self.fraction)
            .from(self.previous)
            .width(px(WIDTH));
        let footnote = self.footnote.map(|note| {
            div()
                .mt(px(FOOTNOTE_MARGIN))
                .max_w(px(WIDTH))
                .text_center()
                .text_size(UiText::Xs.size(&theme))
                .text_color(muted)
                .child(note)
        });
        div()
            .flex()
            .flex_col()
            .items_center()
            .gap(px(STACK_GAP))
            .child(
                div()
                    .text_size(UiText::Xl.size(&theme))
                    .font_weight(FontWeight::SEMIBOLD)
                    .child(self.headline),
            )
            .child(bar)
            .child(self.caption_row(&theme))
            .children(footnote)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    fn width_of(
        cx: &mut TestAppContext,
        panel: impl Fn() -> WaitingPanel + 'static,
    ) -> f32 {
        f32::from(measure(cx, move |_, _| panel()).size.width)
    }

    #[gpui::test]
    fn every_waiting_screen_is_as_wide_as_its_bar(cx: &mut TestAppContext) {
        let step = MigrationStep::new("Rebuild the search index", 2, 5);
        assert_eq!(width_of(cx, WaitingPanel::starting), WIDTH);
        assert_eq!(
            width_of(cx, move || WaitingPanel::migrating(
                Some(&step),
                Some(0.4),
                0.2
            )),
            WIDTH
        );
        assert_eq!(
            width_of(cx, || WaitingPanel::migrating(None, None, 0.0)),
            WIDTH
        );
    }

    #[gpui::test]
    fn the_migration_screen_adds_the_footnote(cx: &mut TestAppContext) {
        let starting = measure(cx, |_, _| WaitingPanel::starting());
        let migrating = measure(cx, |_, _| WaitingPanel::migrating(None, None, 0.0));
        assert!(migrating.size.height > starting.size.height);
    }
}
