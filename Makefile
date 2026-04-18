PLUGIN_ID      = com.asimone.bluesky-unfurl
PLUGIN_VERSION = 0.1.0
BUNDLE         = bluesky-unfurl.tar.gz

SERVER_DIR     = server
SERVER_DIST    = $(SERVER_DIR)/dist
BINARY         = $(SERVER_DIST)/plugin-linux-arm64
DIST_DIR       = dist

GO      = go
GOFLAGS = -trimpath -ldflags="-s -w"

.PHONY: all build bundle clean

all: bundle

## Compile for linux/arm64 (Raspberry Pi 4B / Mattermost 11.x)
build:
	@mkdir -p $(SERVER_DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY) ./$(SERVER_DIR)

## Package into the .tar.gz bundle Mattermost expects.
## Uses Python to set 0o755 on the binary so the Pi can exec it —
## Windows/Git Bash chmod doesn't persist into tar headers on NTFS.
bundle: build
	python -c "\
import tarfile; \
tf = tarfile.open('$(BUNDLE)', 'w:gz'); \
tf.add('plugin.json', arcname='./plugin.json'); \
info = tf.gettarinfo('$(BINARY)', arcname='./server/dist/plugin-linux-arm64'); \
info.mode = 0o755; \
tf.addfile(info, open('$(BINARY)', 'rb')); \
tf.close(); \
print('Bundle ready: $(BUNDLE)')"

## Remove build artefacts
clean:
	rm -rf $(SERVER_DIST) $(DIST_DIR) $(BUNDLE)

## Fetch / tidy Go dependencies (run once after clone)
deps:
	$(GO) mod tidy
