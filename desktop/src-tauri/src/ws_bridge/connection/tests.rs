use super::*;
use crate::ws_bridge::open_bridge;
use crate::ws_bridge::test_support::{
    accept_send_binary_then_idle, accept_then_close, in_own_process, open_fds, sentinel_sink,
    silent_sink, spawn_wedged_daemon, spawn_wedged_daemon_multi, test_socket, ClosureSink,
};
use tokio::net::UnixListener;

#[tokio::test]
async fn daemon_initiated_close_retires_the_connection() {
    let _serialised = crate::test_support::fd_tests().await;

    let sock = test_socket("retire");
    let listener = UnixListener::bind(&sock).unwrap();
    let manager = WsBridgeManager::new();

    let (tx, rx) = oneshot::channel();
    let opened = open_bridge(
        &sock,
        "c1".to_string(),
        "/v0/x".to_string(),
        sentinel_sink(tx),
        &manager,
        "main".to_string(),
        None,
    );
    let (opened, _) = tokio::join!(opened, accept_then_close(&listener));
    opened.unwrap();

    rx.await.expect("the shim must be told the daemon closed");

    assert!(
        manager.connections.lock().unwrap().is_empty(),
        "the daemon closed the stream, so the connection must be retired; a connection \
         left in the map strands its writer task on the socket's write half and burns \
         one of the app's descriptors for good"
    );

    let _ = std::fs::remove_file(&sock);
}

/// Daemon-initiated closes must not leak descriptors (215 in one window broke a 256 limit). Only
/// the descriptor table can show it: a stranded writer still lets its reader finish, so nothing
/// local to the connection reveals the leak.
#[tokio::test]
async fn daemon_initiated_closes_do_not_leak_file_descriptors() {
    let name = "ws_bridge::connection::tests::daemon_initiated_closes_do_not_leak_file_descriptors";
    if !in_own_process(name) {
        return;
    }
    let _serialised = crate::test_support::fd_tests().await;

    const CYCLES: usize = 20;

    let sock = test_socket("fds");
    let listener = UnixListener::bind(&sock).unwrap();
    let manager = WsBridgeManager::new();
    let mut steady_state = 0;

    // Every cycle is one reconnect: the shim opens a fresh conn_id, the daemon closes it.
    for i in 0..CYCLES {
        let (tx, rx) = oneshot::channel();
        let opened = open_bridge(
            &sock,
            format!("c{i}"),
            "/v0/x".to_string(),
            sentinel_sink(tx),
            &manager,
            "main".to_string(),
            None,
        );
        let (opened, _) = tokio::join!(opened, accept_then_close(&listener));
        opened.unwrap();
        rx.await.unwrap();

        // After the first cycle the runtime's own descriptors are all allocated, so this
        // is the level a leak-free bridge must return to every time.
        if i == 0 {
            steady_state = open_fds();
        }
    }

    let after = open_fds();
    assert!(
        after <= steady_state,
        "{CYCLES} daemon-initiated closes leaked file descriptors: {steady_state} -> {after}. \
         Once the app's limit is reached the daemon socket cannot be dialled at all — which \
         the health watchdog reads as a dead backend and kills a daemon that was never \
         unhealthy."
    );

    let _ = std::fs::remove_file(&sock);
}

/// Against a daemon that stopped reading, the retire half-close is never answered, so only explicit
/// cancellation ends the reader parked on `read.next()`, or this hangs. The reader finishing is the
/// assertion: it awaits the writer, so both halves are dropped (no racy descriptor count needed).
#[tokio::test]
async fn explicit_close_releases_the_socket_against_a_daemon_that_never_reacts() {
    let _serialised = crate::test_support::fd_tests().await;

    let sock = test_socket("close");
    spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
    let manager = WsBridgeManager::new();

    let reader = open_bridge(
        &sock,
        "c1".to_string(),
        "/v0/x".to_string(),
        silent_sink(),
        &manager,
        "main".to_string(),
        None,
    )
    .await
    .unwrap();

    manager.connections.lock().unwrap().remove("c1");

    reader
        .await
        .expect("retiring a connection must end its reader, whatever the daemon does");

    let _ = std::fs::remove_file(&sock);
}

/// A page load orphans every connection the old page owned, and a `Channel` keeps working across
/// a reload, so `close_for_window` is all that stops a reload stranding a socket for good.
#[tokio::test]
async fn a_page_load_retires_the_connections_a_reloaded_page_abandoned() {
    let _serialised = crate::test_support::fd_tests().await;

    let sock = test_socket("reload");
    spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
    let manager = WsBridgeManager::new();

    let reader = open_bridge(
        &sock,
        "c1".to_string(),
        "/v0/x".to_string(),
        silent_sink(),
        &manager,
        "main".to_string(),
        None,
    )
    .await
    .unwrap();

    manager.close_for_window("main");

    reader.await.expect("a page load must end its readers");

    assert!(manager.connections.lock().unwrap().is_empty());

    let _ = std::fs::remove_file(&sock);
}

/// The multi-window invariant: a page load in ONE window must not touch another
/// window's connections. Before window scoping this was `close_all`, so window B
/// merely opening — or reloading — silently stranded every socket window A owned.
#[tokio::test]
async fn a_page_load_retires_only_the_loading_windows_connections() {
    let _serialised = crate::test_support::fd_tests().await;

    let sock = test_socket("window-scope");
    spawn_wedged_daemon_multi(UnixListener::bind(&sock).unwrap());
    let manager = WsBridgeManager::new();

    let main_reader = open_bridge(
        &sock,
        "main-conn".to_string(),
        "/v0/x".to_string(),
        silent_sink(),
        &manager,
        "main".to_string(),
        None,
    )
    .await
    .unwrap();

    let w2_reader = open_bridge(
        &sock,
        "w2-conn".to_string(),
        "/v0/x".to_string(),
        silent_sink(),
        &manager,
        "w2".to_string(),
        None,
    )
    .await
    .unwrap();

    manager.close_for_window("w2");

    w2_reader
        .await
        .expect("the loading window's own reader must end");

    {
        let conns = manager.connections.lock().unwrap();
        assert!(
            conns.contains_key("main-conn"),
            "window `main`'s connection must survive window `w2`'s page load"
        );
        assert!(
            !conns.contains_key("w2-conn"),
            "window `w2`'s own connection must be retired by its page load"
        );
    }

    // Clean up the surviving connection so the test leaves no live socket behind.
    manager.close_for_window("main");
    main_reader.await.expect("teardown must end the reader");

    let _ = std::fs::remove_file(&sock);
}

/// The terminal's PTY output is binary: it must reach the webview as the exact
/// bytes, not re-encoded as text.
#[tokio::test]
async fn binary_frames_are_forwarded_as_raw_bytes() {
    let _serialised = crate::test_support::fd_tests().await;

    let sock = test_socket("binary");
    let listener = UnixListener::bind(&sock).unwrap();
    let payload = vec![0u8, b'h', b'i', 0xF0, 0x9F, 0x9A, 0x80];
    tokio::spawn(accept_send_binary_then_idle(listener, payload.clone()));
    let manager = WsBridgeManager::new();

    let (tx, rx) = oneshot::channel();
    let tx = Mutex::new(Some(tx));
    let reader = open_bridge(
        &sock,
        "c1".to_string(),
        "/v0/x".to_string(),
        ClosureSink(move |frame: Frame| {
            if let Some(tx) = tx.lock().unwrap().take() {
                let _ = tx.send(frame);
            }
        }),
        &manager,
        "main".to_string(),
        None,
    )
    .await
    .unwrap();

    assert_eq!(rx.await.unwrap(), Frame::Binary(payload));

    manager.connections.lock().unwrap().remove("c1");
    reader.await.unwrap();
    let _ = std::fs::remove_file(&sock);
}

/// A HALF-OPEN socket delivers nothing while the daemon still lists the session as live; the
/// read-idle timeout retires it and announces it like a daemon close, so the frontend
/// re-attaches instead of typing into a corpse.
#[tokio::test]
async fn a_silent_half_open_socket_is_retired_and_announced() {
    let _serialised = crate::test_support::fd_tests().await;

    let sock = test_socket("halfopen");
    spawn_wedged_daemon(UnixListener::bind(&sock).unwrap());
    let manager = WsBridgeManager::new();

    let (tx, closed) = oneshot::channel();
    let reader = open_bridge(
        &sock,
        "c1".to_string(),
        "/v0/x".to_string(),
        sentinel_sink(tx),
        &manager,
        "main".to_string(),
        Some(Duration::from_millis(50)),
    )
    .await
    .unwrap();

    reader
        .await
        .expect("a silent half-open socket must be judged dead by the read-idle timeout");
    closed
        .await
        .expect("a half-open socket must be announced so the frontend re-attaches");
    assert!(
        manager.connections.lock().unwrap().is_empty(),
        "a half-open connection must be retired, not left holding its socket"
    );

    let _ = std::fs::remove_file(&sock);
}
