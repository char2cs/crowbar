//! GTK's settings: the caret blink `WebKitGTK` pages blink at too, and the colour scheme GTK's
//! menus, dialogs and choosers draw in.

use std::sync::Arc;
use std::time::Duration;

use gtk::prelude::*;

use crate::backend::desktop::caret_blink::{CaretBlink, CaretReport};
use crate::backend::desktop::color_scheme::ColorScheme;
use crate::backend::desktop::desktop_settings::DesktopSettings;
use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::error::Result;

const BLINK: &str = "gtk-cursor-blink";
const BLINK_TIME: &str = "gtk-cursor-blink-time";
const PHASES_PER_CYCLE: u64 = 2;

impl DesktopSettings for GtkPlatform {
    fn follow_caret(
        &self,
        report: CaretReport,
    ) -> Result<()> {
        self.post(move |_| follow(&report))
    }

    fn draw_widgets_in(
        &self,
        scheme: ColorScheme,
    ) -> Result<()> {
        self.post(move |_| prefer(scheme))
    }
}

fn prefer(scheme: ColorScheme) {
    let Some(settings) = gtk::Settings::default() else {
        return;
    };
    let dark = scheme == ColorScheme::Dark;
    if settings.is_gtk_application_prefer_dark_theme() == dark {
        return;
    }
    settings.set_gtk_application_prefer_dark_theme(dark);
}

fn follow(report: &CaretReport) {
    let Some(settings) = gtk::Settings::default() else {
        return;
    };
    report(blink_of(&settings));
    for property in [BLINK, BLINK_TIME] {
        watch(&settings, property, Arc::clone(report));
    }
}

fn watch(
    settings: &gtk::Settings,
    property: &str,
    report: CaretReport,
) {
    settings.connect_notify_local(Some(property), move |settings, _| {
        report(blink_of(settings));
    });
}

fn blink_of(settings: &gtk::Settings) -> CaretBlink {
    if !settings.is_gtk_cursor_blink() {
        return CaretBlink::Steady;
    }
    let cycle = u64::try_from(settings.gtk_cursor_blink_time()).unwrap_or_default();
    CaretBlink::Every(Duration::from_millis(cycle / PHASES_PER_CYCLE))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn following_the_caret_is_posted_to_the_gtk_thread() {
        let (jobs, inbox) = async_channel::unbounded();
        let platform = GtkPlatform {
            jobs,
            stopped: None,
        };
        assert!(platform.follow_caret(Arc::new(|_| ())).is_ok(), "posted");
        assert_eq!(inbox.len(), 1, "one job waits for the GTK thread");
    }

    #[test]
    fn the_widgets_scheme_is_posted_to_the_gtk_thread() {
        let (jobs, inbox) = async_channel::unbounded();
        let platform = GtkPlatform {
            jobs,
            stopped: None,
        };
        assert!(
            platform.draw_widgets_in(ColorScheme::Dark).is_ok(),
            "posted"
        );
        assert_eq!(inbox.len(), 1, "one job waits for the GTK thread");
    }
}
