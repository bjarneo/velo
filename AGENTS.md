# velo for coding agents

This file tells coding agents how to install velo for a user, how to read its results, and how to work on the code.

## Install velo for a user

1. Check the system:

   ```sh
   uname -sm
   ```

   Release builds exist for `Linux x86_64` and `Linux aarch64`. On other systems, use step 4.

2. Install the latest release to `~/.local/bin`:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/bjarneo/velo/main/install.sh | sh
   ```

   The script verifies the SHA-256 checksum. To use another folder, set `VELO_INSTALL_DIR`. To install one release, set `VELO_VERSION`, for example `v0.1.0`.

3. Verify the install:

   ```sh
   velo --version
   ```

   The output is `velo <version>`. When the shell cannot find `velo`, add `~/.local/bin` to `PATH`.

4. When step 2 is not possible, install with Go 1.26 or later:

   ```sh
   go install github.com/bjarneo/velo@latest
   ```

5. On Omarchy, add velo to the app launcher:

   ```sh
   omarchy-tui-install velo velo tile https://raw.githubusercontent.com/bjarneo/velo/main/assets/icon.png
   ```

### Manual install

1. Download `velo_<version>_linux_<arch>.tar.gz` and `checksums.txt` from <https://github.com/bjarneo/velo/releases/latest>.
2. Verify the archive:

   ```sh
   grep " velo_<version>_linux_<arch>.tar.gz$" checksums.txt | sha256sum -c -
   ```

3. Extract `velo` and move it to a folder in `PATH`.

### Uninstall

```sh
rm ~/.local/bin/velo
rm -r ~/.local/share/velo
omarchy-tui-remove velo
```

The second command deletes the run history. The third command applies only to Omarchy.

## Rules for agents

- velo is an interactive terminal program. It needs a TTY. Without a TTY, velo exits with code 1 and the error `could not open TTY`. Do not start velo from a non-interactive shell. Tell the user to run `velo`.
- A test moves about 1 GB of data on a 500 Mbps link. Ask the user before you start a test.
- Do not run `velo --demo` to measure a network. Its results are simulated.
- When the user shares a screenshot or a recording, suggest `--private`. It masks the IP address.

## Read the results

velo appends each finished run to `$XDG_DATA_HOME/velo/history.jsonl`. The default path is `~/.local/share/velo/history.jsonl`. Each line is one JSON object.

| Field | Type | Content |
| --- | --- | --- |
| `id` | int | The run number. |
| `time` | string | The finish time, RFC 3339. |
| `server` | string | The server sponsor and city. |
| `host` | string | The server host and port. |
| `ping_ms` | float | The idle latency. |
| `jitter_ms` | float | The idle jitter. |
| `down_mbps` | float | The download result. |
| `up_mbps` | float | The upload result. |
| `bloat_ms` | float | The bufferbloat. |
| `loss_pct` | float | The share of lost latency probes. |
| `grade` | string | `A+`, `A`, `B`, `C`, or `D`. |
| `data_mb` | float | The data that the test moved. |

To get the last run, run:

```sh
tail -n 1 ~/.local/share/velo/history.jsonl
```

## Work on the code

| Task | Command |
| --- | --- |
| Build | `go build -o velo .` |
| Test | `go test -race ./...` |
| Lint | `go vet ./...` and `gofmt -l .` |
| Run without network | `go run . --demo` |

- Read [docs/architecture.md](docs/architecture.md) before you change the layout or the engines.
- Read [docs/measurement.md](docs/measurement.md) before you change how velo measures.
- `TestRenderSizes` draws every phase and modal at many sizes. Run it after each layout change.
- Every glyph that velo draws must be one column wide. `Canvas` assumes one cell per rune.
- Write documentation in ASD-STE100 Simplified Technical English.

## Make a release

1. Set `version` in `main.go`.
2. Build the archives:

   ```sh
   for arch in amd64 arm64; do
     CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w" -o dist/velo .
     tar -czf dist/velo_<version>_linux_$arch.tar.gz -C dist velo -C .. README.md LICENSE
   done
   (cd dist && sha256sum *.tar.gz > checksums.txt)
   ```

3. Create the release with the tag `v<version>` and attach the archives and `checksums.txt`.
