//! Setting, resetting and uploading the icon of a space.

use std::path::Path;

use daemonapi::Call;
use serde_json::json;
use sidebar::SpaceIcon;

use crate::live::Files;
use crate::live::PlanError;
use daemonapi::multipart;

const PNG: &str = "image/png";
const JPEG: &str = "image/jpeg";
const WEBP: &str = "image/webp";

fn mime_of(path: &str) -> Result<&'static str, PlanError> {
    let extension = Path::new(path)
        .extension()
        .and_then(std::ffi::OsStr::to_str)
        .map(str::to_ascii_lowercase);
    match extension.as_deref() {
        Some("png") => Ok(PNG),
        Some("jpg" | "jpeg") => Ok(JPEG),
        Some("webp") => Ok(WEBP),
        _ => Err(PlanError::IconType),
    }
}

fn upload(
    files: &dyn Files,
    base: &str,
    path: &str,
) -> Result<Call, PlanError> {
    let mime = mime_of(path)?;
    let bytes = files
        .read(Path::new(path))
        .map_err(PlanError::IconUnreadable)?;
    let name = Path::new(path)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .unwrap_or("icon");
    let (body, boundary) = multipart::file_field("icon", (name, mime), &bytes);
    Ok(Call::put(base.to_owned()).form(body, &boundary))
}

pub(crate) fn calls(
    files: &dyn Files,
    project: &str,
    icon: &SpaceIcon,
) -> Result<Vec<Call>, PlanError> {
    let base = format!("{}/icon", daemonapi::routes::project(project));
    let call = match icon {
        SpaceIcon::Default => Call::delete(base),
        SpaceIcon::Emoji(emoji) => Call::put(format!("{base}/emoji")).json(json!({"emoji": emoji})),
        SpaceIcon::Image(path) => upload(files, &base, path)?,
    };
    Ok(vec![call])
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    #[test]
    fn each_icon_kind_reaches_its_route_and_a_bad_image_is_refused_before_any_call() {
        let files = FakeFiles::default();
        let reset = calls(&files, "p", &SpaceIcon::Default).unwrap();
        assert_eq!(
            (reset[0].method, reset[0].path.as_str()),
            ("DELETE", "/v0/projects/p/icon")
        );
        let emoji = calls(&files, "p", &SpaceIcon::Emoji("🦀".into())).unwrap();
        assert_eq!(emoji[0].path, "/v0/projects/p/icon/emoji");
        assert_eq!(emoji[0].body, Some(json!({"emoji": "🦀"})));
        assert!(calls(&files, "p", &SpaceIcon::Image("a.gif".into())).is_err());
        assert!(calls(&files, "p", &SpaceIcon::Image("/gone.png".into())).is_err());
        files.put("/icons/mark.PNG", b"png");
        let image = calls(&files, "p", &SpaceIcon::Image("/icons/mark.PNG".into())).unwrap();
        assert_eq!(
            (image[0].method, image[0].path.as_str()),
            ("PUT", "/v0/projects/p/icon")
        );
        assert!(
            image[0]
                .headers
                .iter()
                .any(|(name, value)| name == "content-type"
                    && value.starts_with("multipart/form-data; boundary="))
        );
    }
}
