# Dev environment setup

Development happens on macOS. Two Homebrew installs, and everything in `docs/dev.md` and `docs/screenshots.md` works:

    brew install go
    brew install --cask google-chrome

- **Go** 1.26 or later — builds and runs the server and all tooling (`go run`, `go vet`).
- **Google Chrome** — launched headless by the screenshot tool; never needs to be opened by hand. The tool finds it in its standard install location automatically.

No Node, no Docker, and no cloud credentials are needed for local development.
