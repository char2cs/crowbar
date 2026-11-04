# Installing the iOS app on your iPhone (free Apple ID)

Requires iOS 26, Xcode, and a Personal Team (no paid Developer Program).

One-time setup:

1. Xcode > Settings > Accounts: sign in with your Apple ID, then Manage Certificates > + > Apple Development.
2. iPhone: plug in, unlock, tap "Trust This Computer".
3. iPhone: Settings > Privacy & Security > Developer Mode > On (restarts).
4. Install Tailscale on the iPhone and join the same tailnet as the Mac.

Then:

```sh
make ios-doctor   # preflight checklist, prints fixes for anything missing
make ios-device   # build release, sign, install, launch on the connected iPhone
make ios-sim      # same, on the booted simulator (SIM_ID=<udid> to pick one)
```

First launch only: iPhone Settings > General > VPN & Device Management > your Apple ID > Trust, then re-run `make ios-device`.

A free-account profile expires after 7 days; re-run `make ios-device` to reinstall.

Overrides: `APPLE_TEAM_ID=<id>`, `DEVICE_ID=<udid>`. If provisioning says `software.rabbyte.crowbar` is unavailable, use `BUNDLE_ID_SUFFIX=<yourname>` (or `IOS_BUNDLE_ID=<full id>`).
