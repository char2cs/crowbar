#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("register embedded fonts: {0}")]
    Fonts(#[source] gpui::private::anyhow::Error),
}
