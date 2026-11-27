use gpui::{AnyElement, Div, IntoElement, ParentElement, SharedString, Styled, div, px};
use store::{Offer, OfferState};
use ui::Palette;

use crate::look::{FINGERPRINT_PAD, FINGERPRINT_TINT, LABEL_GAP, TEXT};
use crate::stage_widgets::muted;

const LEAD: &str = "In Crowbar on the device, add a host with the address, check the fingerprint matches, then enter the code.";
const MINTING: &str = "Making a code…";
const FAILED: &str = "Couldn’t make a code. Check that device connections are on, then try again.";
const NO_ADDRESS: &str = "This computer’s own address and the listening port.";
const CODE_TEXT: gpui::Pixels = px(24.0);
const SECTION_GAP: gpui::Pixels = px(16.0);

fn value_box(
    palette: &Palette,
    mono: &'static str,
    text: &str,
) -> Div {
    div()
        .p(FINGERPRINT_PAD)
        .rounded_md()
        .border_1()
        .border_color(palette.border())
        .bg(palette.accent().opacity(FINGERPRINT_TINT))
        .text_size(TEXT)
        .font_family(mono)
        .child(SharedString::from(text.to_owned()))
}

fn field(label: &'static str) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(LABEL_GAP)
        .child(div().text_size(TEXT).child(label))
}

fn address_label(count: usize) -> &'static str {
    match count {
        0 | 1 => "Address",
        _ => "Addresses",
    }
}

fn addresses(
    offer: &Offer,
    (palette, mono): (&Palette, &'static str),
) -> Div {
    let label = address_label(offer.addresses.len());
    let field = field(label);
    if offer.addresses.is_empty() {
        return field.child(muted(palette).child(NO_ADDRESS));
    }
    let boxes = offer
        .addresses
        .iter()
        .map(|address| value_box(palette, mono, address));
    field.children(boxes)
}

fn offer_body(
    offer: &Offer,
    (palette, mono): (&Palette, &'static str),
) -> Div {
    let code = value_box(palette, mono, &offer.code)
        .text_size(CODE_TEXT)
        .flex()
        .justify_center();
    let until = format!("Works once, until {}.", offer.until);
    div()
        .flex()
        .flex_col()
        .gap(SECTION_GAP)
        .child(muted(palette).child(LEAD))
        .child(
            field("Pairing code")
                .child(code)
                .child(muted(palette).child(until)),
        )
        .child(addresses(offer, (palette, mono)))
        .child(field("Fingerprint").child(value_box(palette, mono, &offer.fingerprint)))
}

pub(super) fn body(
    state: &OfferState,
    look: (&Palette, &'static str),
) -> AnyElement {
    let (palette, _) = look;
    match state {
        OfferState::Ready(offer) => offer_body(offer, look).into_any_element(),
        OfferState::Minting => muted(palette).child(MINTING).into_any_element(),
        OfferState::Failed => div()
            .text_size(TEXT)
            .text_color(palette.destructive())
            .child(FAILED)
            .into_any_element(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn several_addresses_are_labelled_in_the_plural() {
        let labels = [0, 1, 2].map(address_label);
        assert_eq!(labels, ["Address", "Address", "Addresses"]);
    }
}
