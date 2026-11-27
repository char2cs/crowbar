use crate::uuid::Uuid;

pub(crate) fn file_field(
    field: &str,
    (name, mime): (&str, &str),
    bytes: &[u8],
) -> (Vec<u8>, String) {
    let boundary = format!("crowbar{}", Uuid::random().to_string().replace('-', ""));
    let head = format!(
        "--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{name}\"\r\nContent-Type: {mime}\r\n\r\n"
    );
    let mut body = head.into_bytes();
    body.extend_from_slice(bytes);
    body.extend_from_slice(format!("\r\n--{boundary}--\r\n").as_bytes());
    (body, boundary)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_file_sits_between_the_boundary_and_its_closing_line() {
        let (body, boundary) = file_field("icon", ("a.png", "image/png"), b"DATA");
        let text = String::from_utf8(body).unwrap();
        assert!(text.starts_with(&format!("--{boundary}\r\n")));
        assert!(text.contains("name=\"icon\"; filename=\"a.png\""));
        assert!(text.contains("\r\n\r\nDATA\r\n"));
        assert!(text.ends_with(&format!("--{boundary}--\r\n")));
    }
}
