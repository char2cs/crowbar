use crate::backend::desktop::caret_blink::CaretReport;
use crate::backend::desktop::color_scheme::ColorScheme;
use crate::error::Result;

pub trait DesktopSettings {
    fn follow_caret(
        &self,
        report: CaretReport,
    ) -> Result<()>;

    fn draw_widgets_in(
        &self,
        scheme: ColorScheme,
    ) -> Result<()>;
}
