# Bluesky URL Unfurl

A Mattermost plugin that generates rich previews for Bluesky (`bsky.app`) post URLs — author, post text, images, and engagement stats.

This was vibed into existence, so use at your own risk. I'm technical, but not an expert software engineer by any stretch.

![Screenshot](https://github.com/asimone/mattermost-bsky-plugin/blob/main/assets/screenshot.png?raw=true)

## Features

- Automatically unfurls Bluesky post URLs posted in any channel
- Shows author name, handle, post text, and post date
- Displays the first image from a post; notes the count if there are multiple
- Renders quote posts as a second attachment inline
- Renders link cards (external embeds) with title and thumbnail
- Optional engagement counts (likes, reposts, replies, quotes) — togglable in plugin settings
- No Bluesky account or API key required — uses the public AT Protocol API
- Built for `linux/arm64` (Raspberry Pi 4B)

## Installation

1. Download the latest release: **[bluesky-unfurl.tar.gz](../../raw/main/bluesky-unfurl.tar.gz)**
2. In Mattermost, go to **System Console → Plugins → Plugin Management**
3. Under **Upload Plugin**, select the `.tar.gz` file and click **Upload**
4. Enable the plugin — no configuration required

**Optional:** Toggle engagement counts (likes, reposts, replies) in **System Console → Plugins → Bluesky URL Unfurl**.

## Building from Source

Requires Go 1.21+.

```bash
# Build server binary (linux/arm64)
cd server && GOOS=linux GOARCH=arm64 go build -o dist/plugin-linux-arm64 ./...
```

## Compatibility

- Mattermost Server 9.0.0+
- Server binary: `linux/arm64` (Raspberry Pi 4B)
