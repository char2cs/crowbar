use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
};
use ui::{ProgressBar, Surface, Theme, UiText, font};

use crate::migration_step::MigrationStep;

const WIDTH: f32 = 360.0;

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
            caption: step.map_or("Preparing the upgrade".into(), |step| {
                step.name().to_owned().into()
            }),
            count: step.map(|step| format!("{} / {}", step.index(), step.total()).into()),
            fraction,
            previous,
            footnote: Some("Crowbar opens when this finishes. Please keep it open."),
        }
    }
}

impl WaitingPanel {
    fn caption_row(&self, theme: &Theme) -> impl IntoElement {
        let muted = theme.palette(Surface::Window).muted_foreground();
        let count = self.count.clone().map(|count| {
            div()
                .flex_none()
                .font_family(font::MONO_FAMILY)
                .text_size(UiText::Xs.size(theme))
                .child(count)
        });
        div()
            .flex()
            .justify_between()
            .gap(px(12.0))
            .w(px(WIDTH))
            .text_size(UiText::Sm.size(theme))
            .text_color(muted)
            .child(div().min_w_0().truncate().child(self.caption.clone()))
            .children(count)
    }
}

impl RenderOnce for WaitingPanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let muted = theme.palette(Surface::Window).muted_foreground();
        let bar = ProgressBar::new("startup-progress", self.fraction)
            .from(self.previous)
            .width(px(WIDTH));
        let footnote = self.footnote.map(|note| {
            div()
                .mt(px(10.0))
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
            .gap(px(14.0))
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
#[path = "waiting_panel_tests.rs"]
mod tests;
