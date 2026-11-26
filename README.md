# Molovox Labs

A Windows app for soundboards, video samples and live audio. Mix your microphone with clips, add effects, and record a performance or put one together in the arrangement editor. Keyboard shortcuts and MIDI let you trigger clips while gaming or streaming.

Built by **Molotov**.

Closing or minimizing the window keeps it running in the tray. Tap Alt twice to open Soundboard, or right-click the tray icon to quit.

## Installation

1. Download **MolovoxLabs-<version>-x64-Setup.exe** from [Releases](https://github.com/AmirMolotov/MolovoxLabs/releases) on Windows 10 (1809 or later) or Windows 11, x64.
2. Review and accept the setup terms, choose the installation folder, and optionally check **Create a desktop shortcut**. Setup displays each prerequisite as it runs and installs the components it needs. Keep an internet connection available for missing prerequisites, approve the administrator prompt, and restart Windows if setup asks you to.
3. Open **Molovox Labs** from the Start menu or desktop shortcut. Enable **Open on Windows startup** in Settings to start it in the tray when you sign in.

VB-CABLE is supplied by [VB-Audio](https://vb-audio.com/Cable/). It is donationware and contributions are welcome.

Uninstalling Molovox leaves the prerequisites and your library in place. If setup fails, run it again with logging from an elevated PowerShell window. The most recent prerequisite error is also recorded under the `SetupError` value in `HKLM\SOFTWARE\MolovoxLabs`.

```powershell
.\MolovoxLabs-0.4.9-x64-Setup.exe /log "molovox-install.log"
```

For unattended setup, review the terms first and pass `/quiet /norestart AcceptTerms=1` from an elevated terminal. Exit code `3010` indicates a successful installation that needs a restart.

Older releases ship as a standalone EXE. For those releases or the development EXE, install [WebView2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/#download) and put both [FFmpeg tools](https://www.gyan.dev/ffmpeg/builds/) on your PATH. Install VB-CABLE separately if you need the virtual microphone.

## Send your mic and soundboard to another app

![Animated walkthrough of VB-CABLE routing. Select your physical mic and CABLE Input in Molovox Labs, enable Microphone → outputs, then select CABLE Output as the microphone in the receiving app.](assets/guides/vb-cable-routing.gif)

1. In **Molovox Labs → Settings → Microphones**, select the physical microphone you speak into.
2. Under **Speakers & headphones**, select **CABLE Input (VB-Audio Virtual Cable)**. This is the playback device Molovox sends audio to.
3. Enable **Microphone → outputs** in the top audio bar to add your voice to the soundboard mix. If listening is paused, click **Resume listening**. Leave **Patch bay** disabled for this automatic setup. Turn off **Microphone → outputs** to send clips only.
4. In **Discord, OBS, or another app**, select **CABLE Output (VB-Audio Virtual Cable)** as the **microphone** or **audio input**. Keep the receiving app’s playback on your headphones or speakers.
5. Speak into your mic, then play a soundboard clip. The receiving app’s microphone meter should respond to both.

If the cable devices are missing, install [VB-CABLE](https://vb-audio.com/Cable/), restart Windows, then open Molovox Labs and click **Refresh devices** in Settings.

## A quick look

![Soundboard pads, sound trimming, arrangement editing and Patch bay](assets/guides/features.gif)

*10-second tour using generated demo audio in a separate test profile.*

- **Soundboard:** trigger pads with clicks, keyboard shortcuts or MIDI; organize pads into groups.
- **Audio and video sampler:** trim sound bites, crop video samples and perform with video pads.
- **Arrangement:** place clips on tracks and export audio or video.
- **Patch bay:** route microphones and boards through effects to your outputs, including VST3 effects.
- **Recording:** capture audio or a video-sampler performance.

## Your files

Sounds and settings stay on your computer in `%APPDATA%\QuietLab`. The board names, pad layout and assignments are in `soundboards.json`, and the audio files are under `sounds\`. Video sampler media and recordings are under `sampler\`. Updating the app keeps them. Imported media, recordings and local settings are excluded from Git.

## Building from source

You'll need Go 1.27+, Node.js, Python, and Visual Studio C++ Build Tools with the Windows SDK.

```powershell
.\scriptsuild-windows.ps1 -Run
```

Set `VCVARS64` to your Visual Studio `vcvars64.bat` path if the script can't find it. The script runs the checks and builds one `MolovoxLabs.exe`. Docker is not required. Temporary files and Go build caches go into the ignored `.build/` folder on the project drive; keep free space on that drive.

For distribution, build Windows Setup. The script runs the same application checks and writes a single setup executable to the ignored `release/` directory.

```powershell
.\scripts\build-setup.ps1 -Version 0.4.9
```

Push a version tag such as `v0.4.9` to trigger the GitHub release workflow. It verifies the consent requirement and dependency installation order before publishing the setup executable as the release asset.

Source layout and development rules are in [AGENTS.md](AGENTS.md). The Windows application lives in `cmd/molovoxlabs/`; hand-written interface assets live in `web/` and are embedded at build time. Run Go commands from the repository root with `./...` for checks or `./cmd/molovoxlabs` for the application.

See [third-party notices](THIRD-PARTY-NOTICES.txt) for dependency licences.

If the app exits unexpectedly, check `%APPDATA%/QuietLab/logs/app.log` and `app.previous.log`. These retain startup/errors and unhandled Go crash traces from the current and previous session. A forced termination or native plugin crash may leave no Go trace.
