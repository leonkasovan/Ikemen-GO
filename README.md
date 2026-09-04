
<p align="center">
  <a href="https://ikemen-engine.github.io/">
    <img src="https://github.com/user-attachments/assets/0dcd7ae1-5c9d-44e1-aa32-9ec4b9ed3952" style="width: 699px; alt="IKEMEN GO Logo"/>
  </a>
</p>

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

## Features
Ikemen GO aims for backwards-compatibility on par with M.U.G.E.N version 1.1 Beta, while simultaneously expanding on its features in a variety of ways.

This does not mean bug-for-bug emulation. Behavior may intentionally differ when matching M.U.G.E.N would mean preserving bugs, quirks, or unnecessarily limiting the engine.

Refer to [our wiki](https://github.com/ikemen-engine/Ikemen-GO/wiki) to see a comprehensive list of new features that have been added in Ikemen GO.

## This fork
This repository is a fork of [ikemen-engine/Ikemen-GO](https://github.com/ikemen-engine/Ikemen-GO) that stays synced with upstream `develop` while adding rendering, performance, and build-system improvements on top.

### Renderers
- **Direct3D 11** renderer for Windows (`RenderMode = Direct3D 11`) with HLSL shaders, font rendering, and index buffer support.
- **SDL2 Software** renderer (`RenderMode = SDL2 Software`) — CPU rendering with no GPU required, powered by a parallel software rasterizer (worker-pool row rendering, precomputed lookup tables, palette table caching, scalar blend dispatch).
- **Instanced sprite batching** (`EnableSpriteBatching`) for fewer draw calls and faster sprite-heavy scenes.
- **Framebuffer binding cache** in the OpenGL 3.3 and OpenGL ES 3.2 renderers to avoid redundant state changes.
- **RenderScale** option (`0.5–1.0`) renders internally at a fraction of the window resolution and upscales — ideal for low-power ARM devices (auto-set to `0.75` on armdevice builds).
- Palette atlas optimization with configurable memory/atlas settings, lazy texture creation, deterministic GPU texture release, and optimized font glyph atlases.
- Fixes for D3D11 vsync busy-wait on Intel iGPUs, scissor rect scaling under RenderScale, SDL backbuffer clearing between presents, texture sub-rect uploads, and empty-texture handling.

### Memory & performance
- `HeapMemoryLimit` config with `FreeOSMemory` after loading to keep memory usage in check.
- Pooled vertex, audio, batch, and video pixel buffers to reduce allocation churn.
- Lua table caching for config, motif, and select parameters.
- `pprof` heap debugging support and optional `PerfLog` frame-timing/FPS logging (debug builds only).
- Draw call, sprite batch, and framebuffer-switch statistics for profiling.
- GPU texture memory accounting in debug builds — alive texture count and current/peak GPU bytes reported in the `[Mem]` log.

### Build & platform support
- Fully static Windows build — SDL2, FFmpeg, and libvpx compiled from source, no MinGW/SDL2 DLL dependencies.
- libvpx built from source (VP8/VP9, decoder-only) and linked into FFmpeg so WebM videos with a VP8/VP9 alpha stream play correctly.
- Per-platform build directories, Windows toolchain auto-detection, and architecture-suffixed binary names (`Ikemen_GO.amd64.exe`, `*_debug` for debug builds).
- `make install-remote` to copy the built binary to a device over SSH and `make fetch-log` to pull logs back.
- Android APK build scripts (via Docker or natively), Android 13 support, and `armeabi-v7a` ABI builds.
- Vendored Go packages (OpenGL bindings, SDL2 headers, `beep`, `reisen`) for reproducible offline builds.
- ARM device defaults tuned for low-power handhelds (Mali-G31-class GPUs): 75% render scale, models/shadows/MSAA disabled, sprite batching and VSync enabled.

## Installing
Ready-to-use builds are available in the [releases section](https://github.com/ikemen-engine/Ikemen-GO/releases). Stable releases use tags such as `v1.0.0`, while release candidates use tags such as `v1.0.0-rc.1` and are marked as pre-releases. [Nightly builds](https://github.com/ikemen-engine/Ikemen-GO/releases/tag/nightly) are updated after each commit to `develop` and may be less stable.

## Running
Download the ZIP archive that matches your operating system and extract its contents to your preferred location.

On Windows, double-click `Ikemen_GO.amd64.exe`.
On macOS or Linux, double-click `Ikemen_GO.command`.

## Developing
These instructions are for those interested in developing the Ikemen GO engine itself. Instructions for creating custom stages, fonts, characters and other resources can be found in the community forum.

### Building
For setup and platform-specific steps, see [BUILDING.md](./BUILDING.md).
It covers Windows, Linux (including ARM64), macOS (Apple Silicon and Intel), and Android (APK via Docker).

On **Windows** (MSYS2 MINGW64), a single `make` command builds SDL2, FFmpeg, libvpx, and libxmp
from source and produces a fully statically-linked `Ikemen_GO.amd64.exe` with no external
DLL dependencies (except Windows system DLLs).

On **Linux** and **macOS**, the same Makefile detects your platform and builds a native
binary — `Ikemen_GO.amd64` / `Ikemen_GO.arm64` on Linux, `Ikemen_GO` on macOS — with
SDL2, FFmpeg, libvpx, and libxmp compiled in statically and system libraries linked dynamically.

Use `make config=debug` for a debug build with memory instrumentation, `make install`
to assemble a runnable installation with screenpack assets, or `make install-remote`
to copy the built binary to a device over SSH (e.g. an ARM handheld).
See BUILDING.md for prerequisites and platform-specific details.

### Debugging
In order to run the compiled Ikemen GO executable, you will need to download the [engine dependencies](https://github.com/ikemen-engine/Ikemen-GO-Screenpack) and unpack them into the Ikemen-GO source directory. After that, you can use [Goland](https://www.jetbrains.com/go/) or [Visual Studio Code](https://code.visualstudio.com/) to debug.

## Troubleshooting
If you run into any issues with Ikemen Go, you can report it on our [issue tracker](https://github.com/ikemen-engine/Ikemen-GO/issues). It is recommend to read [this page](https://github.com/ikemen-engine/Ikemen-GO/blob/develop/CONTRIBUTING.md) before submitting a bug report.

## References
- [The original reposity of Ikemen GO.](https://osdn.net/users/supersuehiro/pf/ikemen_go/) This project was forked from this repository due to its original author seemingly abandoning the project.

- [The default motif bundled with the engine.](https://github.com/ikemen-engine/Ikemen-GO-Screenpack) Note that this motif is licensed under CC-BY 3 rather than Ikemen GO's source, which is MIT.

## Name
"Ikemen" is an acronym of:

**い**つまでも **完**成しない **永**遠に **未**完成 **エン**ジン  
**I**tsu made mo **K**ansei shinai **E**ien ni **M**ikansei **EN**gine

## License
Ikemen GO engine is under the MIT License.
Bundled screenpack assets are under Creative Commons licenses.
See [LICENCE.txt](LICENCE.txt) for more details.
This program dynamically links FFmpeg (LGPL v2.1).

The exact corresponding source for the FFmpeg build is provided on the [release page](https://github.com/ikemen-engine/Ikemen-GO/releases/latest) as `src_ffmpeg.tar.gz`. You may rebuild this application against a modified FFmpeg.
