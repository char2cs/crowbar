use webview::{Color, WebViewKind};

use crate::error::Result;
use crate::pane_host::PaneHost;

impl PaneHost {
    /// Sets the colour pane web views show before their page paints: on every live view now and
    /// on every view created later. Browser views keep the engine default so a page without a
    /// background of its own stays white.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a view cannot be reached.
    pub fn set_background(
        &mut self,
        color: Color,
    ) -> Result<()> {
        self.factory.set_background(color);
        self.pool
            .views()
            .filter(|(kind, _)| **kind != WebViewKind::Browser)
            .try_for_each(|(_, handle)| handle.set_background(color))?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use webview::Call;

    use super::*;
    use crate::test_support::*;

    const SURFACE: Color = Color {
        red: 250,
        green: 250,
        blue: 250,
        alpha: 255,
    };

    fn backgrounds(f: &Fixture) -> Vec<Color> {
        f.mock
            .calls()
            .iter()
            .filter_map(|call| match call {
                Call::SetBackground { color, .. } => Some(*color),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_theme_change_recolours_the_views_that_already_exist() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f.host.set_background(SURFACE).unwrap();
        assert_eq!(backgrounds(&f), vec![SURFACE], "the one live view");
    }

    #[test]
    fn a_view_created_after_the_theme_is_known_is_born_in_its_colour() {
        let mut f = fixture();
        f.host.set_background(SURFACE).unwrap();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        assert!(
            matches!(
                f.mock.calls().first(),
                Some(Call::Create { background: Some(born), .. }) if *born == SURFACE
            ),
            "{:?}",
            f.mock.calls()
        );
    }
}
