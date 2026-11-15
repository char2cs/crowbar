//! What a failed probe or pairing means for the user, in plain words: the transport's own text
//! names sockets and OS errors, which are never shown.

use crate::refusal::Refusal;

const KEYCHAIN_MARK: &str = "could not be recorded (keychain: ";
const STILL_LISTED: &str = "revoking the token on the host failed";
const KEYCHAIN_REMEDY: &str =
    "Unlock your keychain or allow Crowbar to use it, then pair again with a new code.";
const WRONG_CODE: &str = "That code didn’t work. It may be mistyped or expired — the host doesn’t say which. Check it, or run crowbar pair on the host again for a new one.";
const TOO_MANY: &str =
    "Too many attempts — wait a minute, or run crowbar pair on the host for a new code.";

fn keychain_cause(text: &str) -> &str {
    let after = text
        .split_once(KEYCHAIN_MARK)
        .map_or("", |(_, after)| after);
    after.split_once(')').map_or(after, |(cause, _)| cause)
}

fn keychain(
    text: &str,
    address: &str,
) -> String {
    let cause = keychain_cause(text);
    if text.contains(STILL_LISTED) {
        return format!(
            "Your keychain couldn’t save the key for {address} ({cause}), and undoing the pairing on the host didn’t go through. No copy of the key was kept, so nothing can use it, but the host may still list this device. {KEYCHAIN_REMEDY}"
        );
    }
    format!(
        "Your keychain couldn’t save the key for {address} ({cause}), so the pairing was undone and the host isn’t left paired with this device. {KEYCHAIN_REMEDY}"
    )
}

fn refused(
    refusal: &Refusal<'_>,
    address: &str,
) -> String {
    match refusal.status {
        "401" => WRONG_CODE.to_owned(),
        "429" => TOO_MANY.to_owned(),
        _ => format!("{address} refused pairing: {}.", refusal.reason),
    }
}

fn unresolved(text: &str) -> Option<&str> {
    let name = text.strip_prefix("cannot resolve ")?;
    name.split(|ch: char| ch.is_whitespace() || ch == ':')
        .next()
}

fn unreachable(text: &str) -> bool {
    text.starts_with("cannot reach ")
        || text.contains("did not answer within")
        || text.contains("did not finish pairing")
        || text.starts_with("pairing with ")
}

fn not_crowbar(text: &str) -> bool {
    text.starts_with("TLS handshake with ") || text.contains("presented no certificate")
}

pub(crate) fn words(
    text: &str,
    address: &str,
    name: &str,
) -> String {
    if text.contains(KEYCHAIN_MARK) {
        return keychain(text, address);
    }
    if let Some(found) = Refusal::of(text) {
        return refused(&found, address);
    }
    if text.contains("identity changed") {
        return format!(
            "The host at {address} no longer presents the fingerprint you confirmed, so nothing was sent to it. Go back and check the address."
        );
    }
    if let Some(host) = unresolved(text) {
        return format!("Couldn’t find {host}. Check the spelling, or use its IP address.");
    }
    if unreachable(text) {
        return format!(
            "Can’t reach {address}. Check the address and port, and that the host is running."
        );
    }
    if not_crowbar(text) {
        return format!(
            "{address} answered, but not as a Crowbar host. Check the address and port."
        );
    }
    format!("Couldn’t pair with {name}. Try again, or run crowbar pair on the host for a new code.")
}

#[cfg(test)]
mod tests {
    use super::*;

    const ADDRESS: &str = "box:3737";

    const CASES: [(&str, &str); 9] = [
        (
            "the host refused pairing (401 Unauthorized): bad",
            WRONG_CODE,
        ),
        ("the host refused pairing (429 Too Many): slow", TOO_MANY),
        (
            "the host refused pairing (500 Oops): disk full",
            "box:3737 refused pairing: disk full.",
        ),
        (
            "host identity changed",
            "The host at box:3737 no longer presents the fingerprint you confirmed, so nothing was sent to it. Go back and check the address.",
        ),
        (
            "cannot resolve nowhere.example: no record",
            "Couldn’t find nowhere.example. Check the spelling, or use its IP address.",
        ),
        (
            "cannot reach box:3737",
            "Can’t reach box:3737. Check the address and port, and that the host is running.",
        ),
        (
            "TLS handshake with box:3737 failed",
            "box:3737 answered, but not as a Crowbar host. Check the address and port.",
        ),
        (
            "pairing with box:3737 could not be recorded (keychain: no secret service); the host list is restored; the host revoked the token",
            "Your keychain couldn’t save the key for box:3737 (no secret service), so the pairing was undone and the host isn’t left paired with this device. Unlock your keychain or allow Crowbar to use it, then pair again with a new code.",
        ),
        (
            "something else",
            "Couldn’t pair with lab. Try again, or run crowbar pair on the host for a new code.",
        ),
    ];

    #[test]
    fn each_failure_reads_as_its_plain_words() {
        for (text, expected) in CASES {
            assert_eq!(words(text, ADDRESS, "lab"), expected, "{text}");
        }
    }

    #[test]
    fn an_undo_that_failed_on_the_host_says_the_host_may_still_list_this_device() {
        let text = "pairing with box:3737 could not be recorded (keychain: locked); the host list is restored; revoking the token on the host failed: timed out";
        assert!(words(text, ADDRESS, "lab").contains("may still list this device"));
    }
}
