# Crowbar

_Bring your own agent. Keep your own context._

The workbench for coding agents. Run Claude Code and Codex side by side on real branches, switch between them mid-conversation without losing a word, and review the result like a pull request that never leaves your machine.

- **Your subscription, our bench.** Claude Code, Codex, whichever you already pay for.
- **Change your mind mid-sentence.** Swap the agent between turns; the branch, the diff and the transcript stay put.
- **Conversations that branch like your code.** A chat can start another chat, and it reports back when it's done.
- **Pull requests, without the pull.** Inline comment threads that agents read and answer, all local.
- **Run it anywhere.** Put the daemon on a box of yours and drive it from your laptop.

Free, open source, no account. For macOS and Linux.

---

```arrow
schema: "arrow@v0"

metadata:
  name: crowbar
  description: >-
    The workbench for coding agents. Claude Code and Codex side by side on real branches,
    switchable mid-conversation, with local pull-request-style review. Free, open source, no account.
  license: "AGPL-3.0-only OR LicenseRef-Commercial"
  url: "https://github.com/char2cs/crowbar"
  maintainers:
    - name: char2cs
      url: "https://char2cs.net"
  credits:
    - name: Rabbyte Software
      url: "https://github.com/rabbytesoftware"
  media:
    icon: "https://raw.githubusercontent.com/char2cs/crowbar/develop/.github/crowbar-icon.png"
    banner: "https://raw.githubusercontent.com/char2cs/crowbar/develop/.github/crowbar-banner.png"
  tags:
    - ide
    - development
    - agents
    - git
    - desktop
    - code-review

targets:
  _service:
    methods:
      add_service:
        available_in: [ready]
        steps:
          - type: run
            title: "Run the Crowbar daemon as a login service"
            command: "curl -fsSL https://raw.githubusercontent.com/char2cs/crowbar/${REF}/scripts/service-install.sh | sh -s -- ${REF}"
            timeout: 5m
      remove_service:
        available_in: [ready]
        steps:
          - type: run
            title: "Remove the Crowbar daemon login service"
            command: "curl -fsSL https://raw.githubusercontent.com/char2cs/crowbar/${REF}/scripts/service-uninstall.sh | sh"
            timeout: 2m

  _linux:
    base: _service
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 2
    expose:
      desktop:
        - name: Crowbar
          path: auto
          categories: [Development, IDE]

  "darwin/*":
    base: _service
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 2
    expose:
      desktop:
        - name: Crowbar
          path: auto
    lifecycle:
      install: &darwin-install
        - type: fetch
          title: "Download Crowbar"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_universal.dmg"
          to: "${INSTALL_PATH}/Crowbar.dmg"
          timeout: 15m
        - type: portable
          title: "Install Crowbar"
          from: "${INSTALL_PATH}/Crowbar.dmg"
          to: "${INSTALL_PATH}/app"
          timeout: 10m
      update: *darwin-install

  "linux/amd64":
    base: _linux
    lifecycle:
      install: &linux-amd64-install
        - type: fetch
          title: "Download Crowbar"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_amd64.AppImage"
          to: "${INSTALL_PATH}/Crowbar.AppImage"
          timeout: 15m
        - type: portable
          title: "Install Crowbar"
          from: "${INSTALL_PATH}/Crowbar.AppImage"
          to: "${INSTALL_PATH}/app"
          timeout: 5m
      update: *linux-amd64-install

  "linux/arm64":
    base: _linux
    lifecycle:
      install: &linux-arm64-install
        - type: fetch
          title: "Download Crowbar"
          url: "https://github.com/char2cs/crowbar/releases/download/${REF}/Crowbar_${REF}_aarch64.AppImage"
          to: "${INSTALL_PATH}/Crowbar.AppImage"
          timeout: 15m
        - type: portable
          title: "Install Crowbar"
          from: "${INSTALL_PATH}/Crowbar.AppImage"
          to: "${INSTALL_PATH}/app"
          timeout: 5m
      update: *linux-arm64-install
```

---

Crowbar is dual-licensed under the AGPL-3.0 or a commercial license.
