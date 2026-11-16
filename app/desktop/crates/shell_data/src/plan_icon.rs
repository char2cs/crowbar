//! Setting, resetting and uploading the icon of a space.

use std::path::Path;

use serde_json::json;
use sidebar::SpaceIcon;

use crate::call::Call;
use crate::multipart;
use crate::plan_error::PlanError;

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
    base: &str,
    path: &str,
) -> Result<Call, PlanError> {
    let mime = mime_of(path)?;
    let bytes = std::fs::read(path).map_err(PlanError::IconUnreadable)?;
    let name = Path::new(path)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .unwrap_or("icon");
    let (body, boundary) = multipart::file_field("icon", (name, mime), &bytes);
    Ok(Call::put(base.to_owned()).form(body, &boundary))
}

pub(crate) fn calls(
    project: &str,
    icon: &SpaceIcon,
) -> Result<Vec<Call>, PlanError> {
    let base = format!("{}/icon", crate::routes::project(project));
    let call = match icon {
        SpaceIcon::Default => Call::delete(base),
        SpaceIcon::Emoji(emoji) => Call::put(format!("{base}/emoji")).json(json!({"emoji": emoji})),
        SpaceIcon::Image(path) => upload(&base, path)?,
    };
    Ok(vec![call])
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_icon_kind_reaches_its_route_and_a_bad_image_is_refused_before_any_call() {
        let reset = calls("p", &SpaceIcon::Default).unwrap();
        assert_eq!(
            (reset[0].method, reset[0].path.as_str()),
            ("DELETE", "/v0/projects/p/icon")
        );
        let emoji = calls("p", &SpaceIcon::Emoji("🦀".into())).unwrap();
        assert_eq!(emoji[0].path, "/v0/projects/p/icon/emoji");
        assert_eq!(emoji[0].body, Some(json!({"emoji": "🦀"})));
        assert!(calls("p", &SpaceIcon::Image("a.gif".into())).is_err());
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("mark.PNG");
        std::fs::write(&file, b"png").unwrap();
        let image = calls("p", &SpaceIcon::Image(file.to_string_lossy().into_owned())).unwrap();
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
