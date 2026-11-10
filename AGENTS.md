# Molovox Labs development rules

## Project and user data

- This repository, `F:\projects\molovoxlabs`, is the canonical project. The former `F:\projects\quiet-lab` path is obsolete. Do not mirror changes into the earlier visualization workspace.
- Keep exactly one application executable in the tree: `MolovoxLabs.exe` at the root. Do not create release variants, duplicate executables, or portable ZIPs containing old builds.
- Keep the user's library and preferences under `%APPDATA%\QuietLab`. The product name does not change this compatibility path. Never remove, replace, or migrate sounds as part of a build or refactor.
- Use an explicitly named test profile with `--data-dir` for manual tests. Keep footage, recordings, and browser data out of the normal library. Automated storage tests use `t.TempDir()`.
- Track source, icon assets, `native/bin/MolovoxVST3.dll`, and the application's icon resource. Ignore profiles, recordings, credentials, and other build outputs.

## Where files belong

| Path | Responsibility |
| --- | --- |
| `cmd/molovoxlabs/` | Windows Go application and adjacent Go tests; `package main` |
| `cmd/molovoxlabs/main_windows.go` | Startup, authenticated local server, WebView and native bridge bindings |
| `cmd/molovoxlabs/*_store_windows.go` | Profile persistence for the corresponding feature |
| `cmd/molovoxlabs/native_audio_windows.go`, `mic_filters_windows.go`, `patchbay_windows.go`, `vst_host_windows.go` | Native audio engine, effects, routing validation and plugin hosting |
| `cmd/molovoxlabs/sampler_*`, `arrangement_*`, `board_*` | Existing sampler, arrangement and soundboard responsibilities |
| `cmd/molovoxlabs/desktop_*`, `window_icon_*`, `triggers_*` | Windows lifecycle, tray, icon and input integration |
| `cmd/molovoxlabs/resource_windows_amd64.syso` | Tracked icon resource linked into the Windows command |
| `web/*.html` | Browser entry points; `studio.html` is the desktop studio |
| `web/js/` | Hand-written JavaScript; feature files retain their existing `studio-<feature>` naming |
| `web/css/` | Shared and feature styles with matching names |
| `web/images/` | Runtime icons and SVGs |
| `web/assets.go` | Explicit public asset embed list; never embed Go source or profile data |
| `assets/` | Original artwork and Windows icon inputs |
| `native/vst3/` | C++ VST3 bridge, fixture source and native build command |
| `native/bin/` | Tracked embedded bridge DLL and its Windows embedding file; compiler intermediates ignored |
| `native/test/` | Generated native test fixture, ignored |
| `tests/*.test.cjs` | Node behavior tests for interface features |
| `tests/browser/` | Browser smoke fixtures and their local server |
| `scripts/` | Build, validation and explicit maintenance tools |
| `third_party/` | Vendored VST3 interfaces and licence files |
| `release/` | Ignored local test output; never a second application distribution |

Keep the root for project instructions, module/container configuration, notices, artwork/source folders and the single executable. `web/` is source, not generated output; there is no frontend bundler.

## Patterns and boundaries

- Make the smallest complete change. Read the affected implementation, callers, tests, fixtures, build paths and exports before editing. Preserve behavior during file moves.
- Follow existing feature files and naming. Go methods on `appStore` and `nativeEngine` stay in their current package: Go does not allow adding receiver methods from another package. Extract a package only when it has an actual independent boundary and callers that justify it.
- Keep startup and bridge registration in `main_windows.go`; put feature logic beside its existing store, engine or HTTP implementation. Do not add feature implementations to startup just because it is convenient.
- Reuse the existing store, decoder, loading UI, trim timeline, shortcut helpers and routing validators. Before adding a helper, search for the behavior already present. Avoid generic `utils`, wrapper layers, factories, speculative interfaces and new dependencies for a few lines.
- Keep interface modules organized by feature. Preserve the ordered classic-script loading in HTML and the CommonJS exports used by Node tests. Do not introduce a framework, bundler, module conversion or global-state rewrite as incidental cleanup.
- Browser script and worklet URLs resolve against the page: use `js/<file>.js`. HTML styles use `css/<file>.css`, images use `images/<file>`, and stylesheet image URLs use `../images/<file>`.
- Register a new script once in the appropriate HTML entry point. `scripts/verify-studio.py` derives the studio script list from HTML; do not maintain a second list there.
- Update every affected import, embed pattern, HTML/CSS/dynamic URL, fixture path, test, script and Docker path together when moving files. Do not leave compatibility copies behind.
- Remove superseded release packages and disposable QA output after checks. Keep only the root application executable. Do not delete source, native fixtures needed by tests, vendored headers, or user media merely because they lack a direct text reference.
- Keep `//go:build` constraints and `_windows.go` / `_windows_amd64.go` suffixes appropriate to platform-specific code. Run `gofmt` on changed Go files.

## Safety and behavior that must survive refactors

- Validate external paths, filenames, uploads, imported archives, profile data and plugin inputs at their existing trust boundaries. Preserve traversal checks, limits, cancellation and error handling.
- Preserve atomic profile writes and recovery of interrupted recordings. Handle errors that could lose user data; do not silently discard them.
- Bind the local server to loopback and preserve the random session prefix, origin checks and security headers. Serve only the explicit embedded web assets.
- Effects belong to the Patch bay effects library and connect to microphones or soundboards in Patch bay. Reject feedback cycles before changing the audio graph.
- Video sampler elements remain muted: pad and preview audio use the native engine. Performance recordings combine the selected native output mix from the bounded PCM tap with the canvas stage. Do not substitute microphone or screen capture.
- Preserve native hardware calibration, buffer bounds, synchronization and realtime processing behavior. File organization is not a reason to change audio algorithms.
- Keep keyboard support, focus behavior, accessible names and status announcements when changing controls.
- Document only non-obvious reasons. A deliberate limited shortcut uses `shortcut: <limit>, <when to upgrade>`; do not add speculative features to avoid a hypothetical limit.

## Checks and build

Run commands from the repository root. Use the existing tools; do not add a second build pipeline.

```powershell
.\scripts\build-windows.ps1
```

The build runs `native/vst3/build.cmd`, `go test ./...`, all `tests/*.test.cjs`, and `python scripts/verify-studio.py`, then builds `./cmd/molovoxlabs` into a temporary `.tmp` file before replacing the single root executable. The native command regenerates the bridge, VST fixture and tracked icon resource. Set `VCVARS64` if Visual Studio C++ tools are installed elsewhere. Use `-Run` to open the result.

- For targeted development: `go test ./cmd/molovoxlabs -run <TestName>`, `node --test tests/<feature>.test.cjs`, and `python scripts/verify-studio.py`. Run `go vet ./...` for Go changes.
- Native fixtures must exist before native Go tests: run `cmd /c native\vst3\build.cmd` first in a fresh checkout.
- Changes to routing or playback require both native audio/routing tests and JavaScript behavior tests. File moves require the complete build so embedding, fixtures and icon linkage are checked too.
- Add a small regression test for changed nontrivial behavior or a discovered bug. Reuse existing tests for pure moves; avoid tests that only mirror implementation details.
- The validator checks local page/style resources, studio controls, native bindings and key audio markup. The Go asset test requests every embedded file and checks private source is unavailable. Keep these checks working when extending the interface.
- Report what changed, checks actually run, and remaining manual or hardware checks. Never claim hardware or GUI verification from unit tests alone.
