# Molovox Labs project rules

- This folder, `F:\projects\quiet-lab`, is the canonical project. Make source changes here. Do not mirror changes into the earlier Codex visualization workspace.
- Keep exactly one application executable in this folder tree: `MolovoxLabs.exe` at the project root. Do not create named release variants, duplicate executable copies, or portable ZIPs containing old builds.
- Use `scripts/build-windows.ps1` to build. It compiles to a temporary `.tmp` file, then replaces the one executable.
- Keep the user's library and preferences under `%APPDATA%\QuietLab`; do not remove or replace their sounds when updating the app.
- Effects are defined in the Patch bay effects library and can be connected to microphones or soundboards in Patch bay. Feedback cycles must be rejected before changing the audio graph.
- Run native audio/routing tests and the JavaScript behavior tests for changes to routing or sound playback. Native VST3 fixtures are built by `native/vst3/build.cmd`.
- Track source, icon assets, the embedded bridge DLL and the icon resource in Git. Ignore user data and build outputs.
- Video sampler video elements stay muted: pad/preview audio must use the native engine. Performance recordings capture the selected native output mix through the bounded PCM tap, plus the canvas video stage. Do not use microphone or screen capture as a substitute for that mix.
- Test imported videos, sampler pads and recordings in an explicitly named test profile. Keep test footage out of the user's normal library.
