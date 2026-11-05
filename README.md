# Quiet Lab

Built by **Molotov**.

Open **QuietLab.exe** in this folder. It is the only application executable. No login or Docker is required. Windows x64 and Microsoft Edge WebView2 Runtime are required. WASAPI capture, mixing, built-in effects, global shortcuts and MIDI run in Go. VST3 effects use the embedded native host; WebView2 displays the interface.

## Soundboards

Choose a board, then Add sound to copy WAV or MP3 clips into your local library. Click a pad to play; click a playing pad to stop it. Pads show waveform progress and remaining source time. Each sound accepts multiple global keyboard and MIDI triggers. Sound triggers apply only to the selected board.

Board options exports/imports `.qlboard` packages containing the original audio files and assignments. Individual source audio can also be exported from a sound's shortcut editor. Delete this board asks for confirmation, removes copied audio and shortcuts, and stops its clips. Original files, exports and audio shared with another board are preserved. Deleting the last board creates a new empty board.

## Video sampler

The Video sampler tab has a Clip editor, Video pads, and Recordings.

Import a local MP4, WebM, MOV or M4V source video (up to 200 MB). Source videos are kept in the app's library. Use the playhead and Set in / Set out controls to choose a 0.05–120 second sample. Drag/resize the crop box or enter crop percentages. Name it, then Add sample to this board. Video/audio codecs must be supported by WebView2; H.264/AAC MP4 and VP8/Opus WebM are typical compatible inputs. Audio extraction currently supports source videos up to ten minutes. Silent video can be sampled without decoding an audio track.

Video pads show thumbnails, live video and progress. The latest triggered video appears on the performance stage; audio from all playing samples mixes together. Keyboard/MIDI assignments work for the selected video board alongside the selected audio soundboard. Audio is trimmed to mono 48 kHz WAV and played through the native engine, so device choices, volume and Patch bay effects also apply to video samples. Video elements stay muted to prevent duplicate browser audio. Source/crop references preserve the original video; removing a pad does not delete its source.

On Video pads, choose the native output mix to record, optionally name the performance, and press Record performance. Stop and save writes a WebM recording containing the video stage and that output's processed audio, including routed microphones, soundboards and effects. Recording is limited to 15 minutes / 500 MB. Use Recordings to preview, Save as, or Make samples from a saved performance. Recordings use streamed chunks rather than keeping a full performance in memory. Interrupted recordings with saved chunks are recovered on the next launch.

This records the sampler performance, not the desktop or webcam. Existing .qlboard packages export audio boards; video sampler boards currently depend on the local source library. Export video content by recording a performance. Sources and recordings are stored under %APPDATA%/QuietLab/sampler. All HTTP media/upload routes are restricted to the current local app session and same-origin writes.

Browser checks in tests/browser/sampler-smoke.html verify real PCM worklet audio, WebM encoding, video/audio decoding and cropped thumbnails. Run its small localhost server for the interactive check. Native tests verify stored sample ranges/crops, media range requests, upload boundaries, selected post-effect audio capture, recordings, recovery and shortcut scope.

## Effects

Select a built-in effect from the dropdown to add it immediately. Effects include gain, high-pass, low-pass, gate, compressor, pitch, noise reduction, added white noise, reverb, echo, chorus, distortion and robot. Define up to eight effect instances, reorder them, adjust their settings, and assign keyboard/MIDI toggles to individual effects, groups or the entire effect chain.

When Patch bay routing is off, the effect list processes the microphone and soundboards play dry. When Patch bay is on, cable connections determine which sources each effect processes. You can create two instances of the same effect with different settings.

## Patch bay

Use the **Patch bay** tab to connect sources, effect instances and outputs with cables.

1. Define the desired effects in Effects.
2. Select **Soundboards → effects** for a ready-to-use layout, or enable **Use patch bay routing** and add nodes yourself.
3. Drag a right-hand output socket to a left-hand input socket. Clicking the two sockets also connects them; keyboard users can activate the sockets in sequence.
4. Click a cable to unplug it. Right-click or press Escape to cancel an unfinished cable or node move. Drag node headers to arrange the rack. Node positions and cables are remembered automatically.

Quick Start layouts stay linked to the Effects library: adding, removing or reordering effects updates the active layout automatically. Manual cable/node edits switch to a custom layout, which is preserved. Older recognizable Quick Start layouts are migrated automatically.

Sources can be the microphone mix, all soundboards, or a particular board. Output nodes can feed all selected devices or one specific selected device, such as CABLE Input or monitoring headphones. Select physical devices in Settings first. Multiple cables into an effect sum their sources. An effect's output can branch to multiple destinations. Each effect instance is placed once; create another definition if independent processing is needed.

The top microphone switch still mutes the microphone source, and master output/clip volumes still apply. Each source sample and effect node is evaluated once per device/frame; branching does not advance clip playback twice. Reverb and echo tails may continue after a source pad's progress ends. Cycles are rejected to prevent feedback loops. Limits: 32 nodes, 96 cables, eight effect instances.

## Devices, MIDI and settings

Changes apply immediately; no Apply button. Microphones, gains, outputs, startup listening, MIDI input and global shortcuts are remembered locally. Multiple microphones are averaged; up to eight outputs can be selected. Separate hardware devices may have different playback timing. Input/output waveforms and levels remain visible across tabs.

For Discord with VB-Cable: select CABLE Input as a Quiet Lab output and CABLE Output as Discord's microphone. Add headphones as another output for monitoring. The app does not install a virtual audio driver.

## VST3

In Effects, use Scan plugins, then select a VST3 effect to add it. The plugin's native editor and a searchable generic Parameters editor are available on its card. Standard Windows VST3 locations and a VST3 folder beside the executable are scanned. Additional folders can be listed in Settings. Installed plugin metadata and plugin states are saved locally; plugins themselves must be installed/licensed separately and are not included in board exports.

Supports Windows x64 VST3 audio effects with one mono/stereo input and one mono/stereo output at 48 kHz. VST2, instruments, plugin MIDI input and sidechains are unsupported. Stereo plugin output is folded into the app's mono signal path. The 128-sample bridge adds about 2.7 ms plus the plugin's reported latency. Pitch adds about 44 ms average delay; spectral noise reduction adds about 11 ms. These effects modify recorded microphone signals; this app does not provide headphone active noise cancellation.

Scanning runs one module per child process with a 20-second timeout and reports failures. Live plugin processing/editors run inside the app, so a faulty live plugin may crash it. A processor instance is used per output; parameter edits propagate to all outputs. Plugin-specific external-resource/non-parameter changes may require restarting an effect to synchronize instances. Plugins requesting I/O or latency changes show a restart notice. Missing/incompatible plugins are explicitly marked bypassed.

## Storage and development

This folder is the canonical source repository. User preferences, soundboards, copied audio and the WebView2 profile are stored separately under `%APPDATA%/QuietLab`. The embedded VST3 bridge is extracted to the local user cache. Updating the executable preserves the library.

Build with **scripts/build-windows.ps1**. It builds native dependencies and the icon resource, runs Go and JavaScript behavior tests, checks the UI, and replaces only `QuietLab.exe`. If that app was running, it is reopened. Pass `-Run` to open it after a first build. Go, Node, Python and Visual Studio C++ build tools with a Windows SDK are required. Set `VCVARS64` if your Visual Studio installation differs from the included default. Icon assets are committed; regenerating them with scripts/make-icon.py requires Pillow.

Source, icon assets, the icon resource and the embedded bridge DLL are tracked in Git; application executables, test output, user data, generated compiler intermediates and old packages are ignored. The Steinberg VST3 interface sources and MIT license are vendored in third_party/vst3/pluginterfaces. Dependency notices are in THIRD-PARTY-NOTICES.txt.

Checks cover native filters, graph routing/fan-in/fan-out, soundboard effects, feedback rejection, saved layouts, waveform progress, board archive audio, deletion boundaries, trigger scope, UI selection/routing behavior and native VST3 processing/state restoration. The installed SerumFX plugin was also checked for audio processing, parameter discovery, state saving and opening its native editor. Physical MIDI-controller operation still needs hardware verification.

Loading feedback: startup and blocking audio/file/plugin operations show a branded loading screen before work begins. Background scanning, saving and audio updates show a persistent activity indicator. Settings requests are serialized and rapid effect/routing edits are coalesced so older updates cannot replace newer choices.
