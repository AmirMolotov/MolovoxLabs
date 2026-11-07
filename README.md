# Molovox Labs

A Windows app for soundboards, video samples and live audio. Mix your microphone with clips, add effects, and record a performance or put one together in the arrangement editor. Keyboard shortcuts and MIDI let you trigger clips while gaming or streaming.

Built by **Molotov**.

## Installation

1. Download **MolovoxLabs.exe** from [Releases](https://github.com/AmirMolotov/MolovoxLabs/releases) and run it on **Windows x64**. No account needed.
2. Install [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) if it isn't already on your computer.
3. Install [FFmpeg 7 or newer](https://ffmpeg.org/download.html) for media importing, previews and exports. Include both `ffmpeg` and `ffprobe` on PATH, or use:

   ```powershell
   winget install --id Gyan.FFmpeg --exact
   ```

   Restart the app after installing.

To send your mix to Discord or another app, install [VB-CABLE](https://vb-audio.com/Cable/). Choose **CABLE Input** as an output in Molovox Labs and **CABLE Output** as Discord's microphone. You can select your headphones too, so you hear the mix. VB-CABLE is optional for normal playback.

## Your files

Sounds and settings stay on your computer in `%APPDATA%/QuietLab`. Updating the app keeps them. Imported media, recordings and local settings are excluded from Git.

## Building from source

You'll need Go 1.27+, Node.js, Python, and Visual Studio C++ Build Tools with the Windows SDK.

```powershell
.\scriptsuild-windows.ps1 -Run
```

Set `VCVARS64` to your Visual Studio `vcvars64.bat` path if the script can't find it. The script runs the checks and builds one `MolovoxLabs.exe`.

See [third-party notices](THIRD-PARTY-NOTICES.txt) for dependency licences.
