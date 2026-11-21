//! A page's `prompt()`, `confirm()` and `alert()` show `WebKitGTK`'s own dialog over the page, as
//! develop's Tauri window does, and the key that confirms it answers the page.

use std::time::Instant;

use x11rb::protocol::xproto::{ConnectionExt as _, Screen};
use x11rb::rust_connection::RustConnection;

use webview::Visibility;

use super::pixels::{DARK_PAGE, Region, shows_before};
use super::{KEY_WAIT, KEYCODE_A, PAGE_WAIT, SIDE, TestResult, page_in, type_key, wait_for};

const WHOLE: Region = (0, 0, SIDE, SIDE);

const KEYCODE_RETURN: u8 = 36;
const DIALOGS: [(&str, &str); 3] = [
    ("prompt('Save as:', 'Untitled')", "answer Untitled"),
    ("confirm('Discard the changes?')", "answer true"),
    ("alert('Saved')", "answer undefined"),
];

fn ask(call: &str) -> String {
    format!("setTimeout(() => window.ipc.postMessage('answer ' + {call}))")
}

pub(super) fn a_dialog_shows_over_its_page_and_answers_it(
    connection: &RustConnection,
    screen: &Screen,
    host: &webview::Host,
    parent: u32,
) -> TestResult {
    let (view, messages) = page_in(host, parent, Visibility::Shown)?;
    view.eval(DARK_PAGE)?;
    wait_for(&messages, "dark", PAGE_WAIT)?;
    view.focus()?;
    for (call, answer) in DIALOGS {
        let gone = shows_before(
            connection,
            (parent, WHOLE),
            |lit| lit == 0,
            Instant::now() + KEY_WAIT,
        )?;
        assert!(gone, "the page is not dark before {call}");
        view.eval(&ask(call))?;
        let shown = shows_before(
            connection,
            (parent, WHOLE),
            |lit| lit > 0,
            Instant::now() + KEY_WAIT,
        )?;
        assert!(shown, "{call} never showed over the page");
        type_key(connection, screen, KEYCODE_RETURN)?;
        assert_eq!(
            wait_for(&messages, answer, KEY_WAIT),
            Ok(()),
            "Return did not answer {call}"
        );
    }
    view.destroy()?;
    Ok(())
}

fn focus_reaches(
    connection: &RustConnection,
    window: u32,
    deadline: Instant,
) -> TestResult<bool> {
    let seen = std::iter::from_fn(|| {
        (Instant::now() < deadline).then(|| connection.get_input_focus()?.reply())
    })
    .find(|focus| focus.as_ref().map_or(true, |reply| reply.focus == window));
    Ok(seen.transpose()?.is_some())
}

pub(super) fn a_prompt_takes_typing_after_the_keyboard_comes_back(
    connection: &RustConnection,
    screen: &Screen,
    host: &webview::Host,
    parent: u32,
) -> TestResult {
    let (view, messages) = page_in(host, parent, Visibility::Shown)?;
    view.eval(DARK_PAGE)?;
    wait_for(&messages, "dark", PAGE_WAIT)?;
    view.focus()?;
    view.eval("window.ipc.postMessage('focused')")?;
    wait_for(&messages, "focused", PAGE_WAIT)?;
    let page = connection.get_input_focus()?.reply()?.focus;
    view.eval(&ask("prompt('Save as:', 'Untitled')"))?;
    let shown = shows_before(
        connection,
        (parent, WHOLE),
        |lit| lit > 0,
        Instant::now() + KEY_WAIT,
    )?;
    assert!(shown, "the prompt never showed over the page");
    view.focus_parent()?;
    assert!(
        focus_reaches(connection, parent, Instant::now() + KEY_WAIT)?,
        "the parent never took the keyboard"
    );
    view.focus()?;
    assert!(
        focus_reaches(connection, page, Instant::now() + KEY_WAIT)?,
        "the page never took the keyboard back"
    );
    type_key(connection, screen, KEYCODE_A)?;
    type_key(connection, screen, KEYCODE_RETURN)?;
    assert_eq!(
        wait_for(&messages, "answer a", KEY_WAIT),
        Ok(()),
        "the prompt lost the typing once the keyboard came back"
    );
    view.destroy()?;
    Ok(())
}
