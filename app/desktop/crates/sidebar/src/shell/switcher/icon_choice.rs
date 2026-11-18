//! Turning what the user typed or picked in the icon editor into a [`SpaceIcon`].

use std::path::Path;

use crate::shell::switcher::space_icon::SpaceIcon;

const IMAGE_EXTENSIONS: [&str; 4] = ["png", "jpg", "jpeg", "webp"];
const EMOJI_MAX_CHARS: usize = 4;

impl SpaceIcon {
    /// An emoji icon from the typed text, trimmed; `None` when it is empty or longer than
    /// four characters.
    pub fn emoji(text: &str) -> Option<Self> {
        let text = text.trim();
        let fits = !text.is_empty() && text.chars().count() <= EMOJI_MAX_CHARS;
        fits.then(|| Self::Emoji(text.to_owned()))
    }

    /// An image icon from the picked file; `None` unless it is a png, jpeg or webp.
    pub fn image(path: &Path) -> Option<Self> {
        let extension = path.extension()?.to_str()?.to_ascii_lowercase();
        IMAGE_EXTENSIONS
            .contains(&extension.as_str())
            .then(|| Self::Image(path.to_string_lossy().into_owned()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_emoji_is_one_to_four_characters() {
        let cases = [
            ("🚀", Some("🚀")),
            ("  🚀  ", Some("🚀")),
            ("abcd", Some("abcd")),
            ("abcde", None),
            ("   ", None),
            ("", None),
        ];
        for (text, want) in cases {
            let want = want.map(|text| SpaceIcon::Emoji(text.to_owned()));
            assert_eq!(SpaceIcon::emoji(text), want, "{text:?}");
        }
    }

    #[test]
    fn only_png_jpeg_and_webp_files_are_images() {
        let cases = [
            ("/a/logo.png", true),
            ("/a/logo.JPG", true),
            ("/a/logo.jpeg", true),
            ("/a/logo.webp", true),
            ("/a/logo.gif", false),
            ("/a/logo", false),
        ];
        for (path, accepted) in cases {
            assert_eq!(
                SpaceIcon::image(Path::new(path)).is_some(),
                accepted,
                "{path}"
            );
        }
    }
}
