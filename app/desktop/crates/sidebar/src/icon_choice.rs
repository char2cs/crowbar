//! Turning what the user typed or picked in the icon editor into a [`SpaceIcon`].

use std::path::Path;

use crate::space_icon::SpaceIcon;

const IMAGE_EXTENSIONS: [&str; 4] = ["png", "jpg", "jpeg", "webp"];
const EMOJI_MAX_CHARS: usize = 4;

pub(crate) fn emoji_icon(text: &str) -> Option<SpaceIcon> {
    let text = text.trim();
    let fits = !text.is_empty() && text.chars().count() <= EMOJI_MAX_CHARS;
    fits.then(|| SpaceIcon::Emoji(text.to_owned()))
}

pub(crate) fn image_icon(path: &Path) -> Option<SpaceIcon> {
    let extension = path.extension()?.to_str()?.to_ascii_lowercase();
    IMAGE_EXTENSIONS
        .contains(&extension.as_str())
        .then(|| SpaceIcon::Image(path.to_string_lossy().into_owned()))
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
            assert_eq!(emoji_icon(text), want, "{text:?}");
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
            assert_eq!(image_icon(Path::new(path)).is_some(), accepted, "{path}");
        }
    }
}
