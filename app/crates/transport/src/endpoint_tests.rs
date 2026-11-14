use super::Endpoint;

#[test]
fn a_host_and_port_parses_and_reads_back_as_written() {
    let long_label = format!("{}.com:1", "a".repeat(63));
    let longest_name = format!("{}a:3737", "a.".repeat(126));
    let valid = [
        "homelab.example.com:3737",
        "HomeLab.Example.com:3737",
        "homelab:4000",
        "my-host-2.lan:3737",
        "xn--bcher-kva.example:3737",
        "localhost:38737",
        "1.2.3.4:3737",
        "0.0.0.0:1",
        "255.255.255.255:65535",
        "[::1]:3737",
        "[fd00::2]:4000",
        "[::ffff:102:304]:3737",
        &long_label,
        &longest_name,
    ];
    for text in valid {
        let endpoint = Endpoint::parse(text).unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(endpoint.to_string(), text);
    }
}

fn assert_refused(refused: &[&str]) {
    for text in refused {
        let err = Endpoint::parse(text).expect_err(text);
        assert!(err.to_string().contains("is not a host:port address"), "{err}");
    }
}

#[test]
fn anything_but_a_host_and_port_is_refused() {
    assert_refused(&[
        "",
        "homelab.example.com",
        "1.2.3.4",
        "[::1]",
        "::1",
        "::1:3737",
        "fd00::2:3737",
        "https://homelab.example.com:3737",
        "homelab.example.com:3737/",
        "homelab.example.com:3737?q",
        "me@homelab.example.com:3737",
        "home lab.example.com:3737",
        " homelab.example.com:3737",
        "home_lab.example.com:3737",
        "-homelab.example.com:3737",
        "homelab-.example.com:3737",
        "homelab..example.com:3737",
        "homelab.example.com.:3737",
        "bücher.example:3737",
    ]);
}

#[test]
fn bad_ports_bad_addresses_and_overlong_names_are_refused() {
    let long_label = format!("{}.com:3737", "a".repeat(64));
    let too_long = format!("{}ab:3737", "a.".repeat(126));
    assert_refused(&[
        "homelab.example.com:",
        "homelab.example.com:0",
        "homelab.example.com:65536",
        "homelab.example.com:+80",
        "homelab.example.com:-1",
        "homelab.example.com:http",
        "homelab.example.com:3737:3737",
        "1.2.3.256:3737",
        "1.2.3:3737",
        "01.2.3.4:3737",
        "host.123:3737",
        "[homelab]:3737",
        "[::1]x:3737",
        "[::1]:3737]",
        "[fe80::1%en0]:3737",
        &long_label,
        &too_long,
    ]);
}
