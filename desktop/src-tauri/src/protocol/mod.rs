use tauri::http::Response;

pub async fn handle(_request: tauri::http::Request<Vec<u8>>) -> Response<Vec<u8>> {
    Response::builder()
        .status(200)
        .body(b"ok".to_vec())
        .unwrap()
}
