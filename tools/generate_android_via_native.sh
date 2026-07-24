#!/usr/bin/env bash
# =============================================================================
# generate_android_via_native.sh — Android SDK/NDK setup + APK build (API 30+)
#
# Downloads, installs, and builds everything needed for Ikemen-GO's Android
# target on Windows (MSYS2 / MINGW64). Produces a ready-to-install APK.
#
# Default target: Android 11 (API 30). Override for other API levels:
#   ./tools/generate_android_via_native.sh --yes                              # Android 11 (API 30)
#   SDK_PLATFORM=android-33 SDK_BUILD_TOOLS=33.0.1 \
#     ./tools/generate_android_via_native.sh --yes                            # Android 13 (API 33)
#   SDK_PLATFORM=android-34 SDK_BUILD_TOOLS=34.0.0 \
#     ./tools/generate_android_via_native.sh --yes                            # Android 14 (API 34)
#
# What gets installed:
#   1.  MSYS2 build tools        (make, cmake, gcc, g++, nasm, pkg-config, etc.)
#   2.  Go 1.22+                 (mingw-w64-x86_64-go via pacman)
#   3.  Eclipse Temurin JDK 17   (for Gradle / sdkmanager)
#   4.  Android NDK r27d          (cross-compiler for arm64-v8a)
#   5.  SDL2 cross-compiled      for Android arm64-v8a (into build/android-deps/)
#   6.  libxmp cross-compiled    for Android arm64-v8a
#   7.  FFmpeg cross-compiled    for Android arm64-v8a (minimal: VP9/Opus/Vorbis)
#   8.  Android SDK cmdline-tools + platform + build-tools
#   9.  Environment variables     (ANDROID_NDK_HOME, JAVA_HOME, etc.)
#   10. libmain.so               (Go c-shared build via NDK clang)
#   11. ikemen-droid source      (downloaded from IKEMEN_DROID_URL → IKEMEN_DROID_SRC)
#   12. Screenpack assets        (downloaded from leonkasovan/Ikemen-GO-Screenpack → deploy/)
#   13. APK build + sign         (ikemen-droid Gradle project → build/ikemen-go.apk)
#
# Prerequisites:
#   - ikemen-droid source is downloaded automatically from IKEMEN_DROID_URL
#     (override with IKEMEN_DROID_SRC to use an existing local checkout)
#   - Screenpack assets are downloaded automatically (override with existing deploy/ dir)
#   - The script builds libmain.so automatically (step 10)
#
# After running:
#   build/ikemen-go.apk            # ready to install on Android device
#
# Usage:
#   ./tools/generate_android_via_native.sh              # interactive (asks before each step)
#   ./tools/generate_android_via_native.sh --yes        # non-interactive, auto-confirm all
#   ./tools/generate_android_via_native.sh --help       # show this header
# =============================================================================

set -euo pipefail

# ────────────────────────────────────────────────────────────────────────────
# Configuration (override via environment variables)
# ────────────────────────────────────────────────────────────────────────────

# Base install directory for Android tools
ANDROID_HOME_DIR="${ANDROID_HOME_DIR:-/c/Android/SDK}"

# Individual component paths (derived from ANDROID_HOME_DIR by default)
NDK_INSTALL_DIR="${NDK_INSTALL_DIR:-${ANDROID_HOME_DIR}/ndk/r27d}"
SDK_INSTALL_DIR="${SDK_INSTALL_DIR:-${ANDROID_HOME_DIR}}"

# Versions
NDK_VERSION="${NDK_VERSION:-r27d}"
SDK_PLATFORM="${SDK_PLATFORM:-android-30}"
SDK_BUILD_TOOLS="${SDK_BUILD_TOOLS:-30.0.3}"
# Extract numeric API level from SDK_PLATFORM (e.g., "android-30" -> "30")
ANDROID_API="${SDK_PLATFORM##android-}"
# Download URLs
IKEMEN_DROID_URL="https://github.com/leonkasovan/ikemen-droid/archive/refs/heads/main.zip"
NDK_URL="https://dl.google.com/android/repository/android-ndk-${NDK_VERSION}-windows.zip"
CMDLINE_TOOLS_URL="https://dl.google.com/android/repository/commandlinetools-win-11076708_latest.zip"
JDK17_URL="https://api.adoptium.net/v3/binary/latest/17/ga/windows/x64/jdk/hotspot/normal/eclipse"
# Note: Go is installed via pacman (mingw-w64-x86_64-go) on MSYS2, not downloaded.
JDK17_INSTALL_DIR="${JDK17_INSTALL_DIR:-/c/Android/jdk-17}"

# SDL2 cross-compilation for Android arm64-v8a
SDL2_VERSION="${SDL2_VERSION:-release-2.32.10}"
SDL2_URL="${SDL2_URL:-https://github.com/libsdl-org/SDL/archive/refs/tags/${SDL2_VERSION}.zip}"
# libxmp cross-compilation for Android arm64-v8a
XMP_VERSION="${XMP_VERSION:-libxmp-4.7.1}"
XMP_URL="${XMP_URL:-https://github.com/libxmp/libxmp/archive/refs/tags/${XMP_VERSION}.zip}"
# FFmpeg cross-compilation for Android arm64-v8a
FFMPEG_VERSION="${FFMPEG_VERSION:-n7.1}"
FFMPEG_URL="${FFMPEG_URL:-https://github.com/FFmpeg/FFmpeg/archive/refs/tags/${FFMPEG_VERSION}.zip}"
# ikemen-droid APK build
IKEMEN_DROID_SRC="${IKEMEN_DROID_SRC:-$(pwd)/ikemen-droid-src}"
IKEMEN_DROID_DIR="${IKEMEN_DROID_DIR:-$(pwd)/build/android-apk/ikemen-droid}"
ANDROID_BINARY="${ANDROID_BINARY:-$(pwd)/android/app/libs/arm64-v8a/libmain.so}"
APK_OUTPUT="${APK_OUTPUT:-$(pwd)/build/ikemen-go.apk}"
ANDROID_GRADLE_TASK="${ANDROID_GRADLE_TASK:-assembleRelease}"
ANDROID_APK_VARIANT="${ANDROID_APK_VARIANT:-release}"
ANDROID_APK_ARTIFACT="${ANDROID_APK_ARTIFACT:-app-release.apk}"
# APK signing — set ANDROID_KEYSTORE to enable; passwords via env or pass: syntax.
# If the file does not exist, the script auto-generates it with 'keytool'
# using ANDROID_KEYSTORE_PASS / ANDROID_KEY_PASS (the 'pass:' prefix is stripped
# for keytool, which expects the raw password).
# WARNING: Do NOT commit passwords to version control. Use env:VAR or file:PATH.
#          Keep the generated keystore safe — a lost key cannot be recovered and
#          the APK can no longer be updated on the Play Store / device.
ANDROID_KEYSTORE="${ANDROID_KEYSTORE:-$(pwd)/android/release.jks}"
ANDROID_KEY_ALIAS="${ANDROID_KEY_ALIAS:-androidkey}"
ANDROID_KEYSTORE_PASS="${ANDROID_KEYSTORE_PASS:-pass:Secret14!}"
ANDROID_KEY_PASS="${ANDROID_KEY_PASS:-$ANDROID_KEYSTORE_PASS}"

# Path where SDL2 will be cross-compiled and installed for Android arm64-v8a.
# Must match ANDROID_DEPS_PATH in the Makefile (build/android-deps).
# We compute it at script start so it's absolute.
ANDROID_DEPS_PATH="${ANDROID_DEPS_PATH:-$(pwd)/build/android-deps}"

# SDK / JDK paths — used by build_apk; may come from .bashrc or set explicitly
ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME_DIR}}"
ANDROID_HOME="${ANDROID_HOME:-${ANDROID_HOME_DIR}}"
JAVA_HOME="${JAVA_HOME:-$JDK17_INSTALL_DIR}"
ANDROID_GCDB_URL="${ANDROID_GCDB_URL:-https://raw.githubusercontent.com/mdqinc/SDL_GameControllerDB/refs/heads/master/gamecontrollerdb.txt}"

# Auto-detect GOROOT for MSYS2 MinGW Go if not already set
if [[ -z "${GOROOT:-}" ]]; then
  _go_bin="$(command -v go 2>/dev/null || true)"
  if [[ -n "$_go_bin" ]]; then
    _go_bin="$(cygpath -u "$_go_bin" 2>/dev/null || echo "$_go_bin")"
    # Go binary is at GOROOT/bin/go; resolve upwards
    _candidate="$(dirname "$(dirname "$_go_bin")")"
    if [[ -d "$_candidate/lib/go" ]]; then
      GOROOT="$_candidate/lib/go"
    elif [[ -d "$_candidate/go" ]]; then
      GOROOT="$_candidate/go"
    elif [[ -d "$_candidate" ]] && [[ -f "$_candidate/bin/go" ]]; then
      GOROOT="$_candidate"
    fi
  fi
  # Fallback: standard MSYS2 MinGW64 path
  if [[ -z "${GOROOT:-}" ]] && [[ -d "/mingw64/lib/go" ]]; then
    GOROOT="/mingw64/lib/go"
  fi
  if [[ -n "${GOROOT:-}" ]]; then
    export GOROOT
  fi
fi
export GOPATH="${GOPATH:-$HOME/go}"
export GOCACHE="${GOCACHE:-$HOME/.cache/go-build}"

# ────────────────────────────────────────────────────────────────────────────
# Help & flags
# ────────────────────────────────────────────────────────────────────────────

if [[ "${1:-}" == "--help" ]]; then
  sed -n '2,20p' "$0"
  exit 0
fi

AUTO_YES=false
if [[ "${1:-}" == "--yes" ]]; then
  AUTO_YES=true
fi

confirm() {
  local msg="$1"
  if $AUTO_YES; then
    echo "  [auto-yes] $msg"
    return 0
  fi
  read -r -p "❓ $msg [Y/n] " reply
  case "${reply,,}" in
    n|no) return 1 ;;
    *)    return 0 ;;
  esac
}

# Check which tools from a list are available on PATH.
# Echoes the missing ones (empty string = all found).
check_tools_on_path() {
  local missing=()
  for tool in "$@"; do
    if ! command -v "$tool" &>/dev/null; then
      missing+=("$tool")
    fi
  done
  echo "${missing[@]}"
}

# Extract the Java major version from a java executable.
# Returns "0" if the JVM cannot be queried.
# Uses awk to parse the version string (more reliable on MSYS2 than bash
# parameter expansion with escaped quotes, and more reliable than sed pat-
# terns which can leak trailing data like "11 2026-04-21" instead of "11").
java_major_version() {
  local java_bin="$1"
  if [[ ! -x "$java_bin" ]]; then
    echo 0
    return
  fi
  "$java_bin" -version 2>&1 | head -n1 | \
    awk -F '"' '{print $2}' | \
    awk -F '.' '{print $1}'
  # On failure, awk produces empty output. The callers handle that gracefully
  # (empty string in numeric context [[ ... -eq 11 ]] evaluates as 0).
}

# ────────────────────────────────────────────────────────────────────────────
# Preflight checks
# ────────────────────────────────────────────────────────────────────────────

echo "╔══════════════════════════════════════════════════════════╗"
echo "║   Ikemen-GO Android 11 — Environment Setup Script      ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# Must be run in MSYS2/MINGW64
case "$OSTYPE" in
  msys|cygwin) ;;
  *)
    echo "ERROR: This script is designed for MSYS2/MINGW64 on Windows."
    echo "  Detected OSTYPE: $OSTYPE"
    echo "  Please launch 'MSYS2 MINGW64' from the Start Menu and re-run."
    exit 1
    ;;
esac

# Verify MINGW64 (not MSYS) shell
if [[ "${MSYSTEM:-}" != "MINGW64" ]]; then
  echo "ERROR: You are in an MSYS shell, not MINGW64."
  echo "  Launch 'MSYS2 MINGW64' from the Start Menu."
  echo "  Current MSYSTEM=$MSYSTEM"
  exit 1
fi

echo "✅  Shell: MSYS2 MINGW64"
echo ""

# ────────────────────────────────────────────────────────────────────────────
# Step 1 — Install MSYS2 build tools (via pacman)
# ────────────────────────────────────────────────────────────────────────────

install_msys2_packages() {
  echo ""
  echo "═══ Step 1/12 — Installing MSYS2 build tools ═══"

  # First check which tools are already available on PATH
  local required_bins=(
    make gcc g++ go cmake nasm pkg-config
    wget unzip
  )
  local missing=( $(check_tools_on_path "${required_bins[@]}") )

  if [[ ${#missing[@]} -eq 0 ]]; then
    echo "  ✅  All tools already present on system PATH."
    echo "         $(make --version 2>&1 | head -n1)"
    echo "         $(gcc --version 2>&1 | head -n1)"
    echo "         $(go version 2>&1)"
    echo "         $(cmake --version 2>&1 | head -n1)"
    echo "         $(nasm --version 2>&1 | head -n1)"
    echo "         $(pkg-config --version 2>&1)"
    return
  fi

  # Map binary names to pacman package names
  local packages=()
  local pkg_map=(
    make:make
    pkg-config:mingw-w64-x86_64-pkg-config
    go:mingw-w64-x86_64-go
    gcc:mingw-w64-x86_64-toolchain
    g++:mingw-w64-x86_64-toolchain
    nasm:mingw-w64-x86_64-nasm
    cmake:mingw-w64-x86_64-cmake
    wget:wget
    unzip:unzip
  )
  for entry in "${pkg_map[@]}"; do
    local bin="${entry%%:*}"
    local pkg="${entry##*:}"
    for m in "${missing[@]}"; do
      if [[ "$m" == "$bin" ]]; then
        packages+=("$pkg")
        break
      fi
    done
  done
  # Deduplicate (gcc and g++ both map to mingw-w64-x86_64-toolchain)
  local dedup=()
  for p in "${packages[@]}"; do
    local seen=false
    for d in "${dedup[@]+${dedup[@]}}"; do [[ "$d" == "$p" ]] && seen=true && break; done
    $seen || dedup+=("$p")
  done

  echo "  Missing on PATH: ${missing[*]}"
  echo "  Packages to install: ${dedup[*]}"

  if ! confirm "Install missing MSYS2 build tools via pacman?"; then
    echo "  Skipped."
    return
  fi

  # Update package databases first
  echo "==> Updating package databases..."
  pacman -Sy --noconfirm

  # Install packages (retry once if mirror is stale)
  echo "==> Installing packages..."
  if pacman -S --noconfirm --needed "${dedup[@]}"; then
    echo "✅  MSYS2 build tools installed."
  else
    echo "==> Retrying with refreshed databases..."
    pacman -Sy --noconfirm
    pacman -S --noconfirm --needed "${dedup[@]}"
    echo "✅  MSYS2 build tools installed (after retry)."
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 2 — Install JDK 17 (Eclipse Temurin)
# ────────────────────────────────────────────────────────────────────────────
# Step 2 — Install JDK 17 (Eclipse Temurin)
# ────────────────────────────────────────────────────────────────────────────

install_jdk17() {
  echo ""
  echo "═══ Step 2/12 — Installing JDK 17 (Eclipse Temurin) ═══"

  # Check if JDK 17 is already on PATH via java -version
  if command -v java &>/dev/null; then
    local major
    major="$(java_major_version "$(command -v java)")"
    if [[ "$major" -ge 17 ]]; then
      echo "  ✅  JDK 17+ (or newer) already on system PATH:"
      echo "         $(java -version 2>&1 | head -n1)"
      echo "         $(command -v java)"
      SDKMANAGER_JAVA_HOME="$(dirname "$(dirname "$(command -v java)")")"
      return
    fi
  fi

  # Fallback: check the canonical install directory
  if [[ -f "$JDK17_INSTALL_DIR/bin/java.exe" ]]; then
    local major
    major="$(java_major_version "$JDK17_INSTALL_DIR/bin/java.exe")"
    if [[ "$major" -ge 17 ]]; then
      echo "  ✅  JDK 17 already installed at: $JDK17_INSTALL_DIR"
      SDKMANAGER_JAVA_HOME="$JDK17_INSTALL_DIR"
      return
    fi
  fi

  if ! confirm "Download and install JDK 17 to $JDK17_INSTALL_DIR? (Required by latest sdkmanager)"; then
    echo "  Skipped — will try to run sdkmanager with available java."
    SDKMANAGER_JAVA_HOME=""
    return
  fi

  local tmp_zip="/tmp/jdk17-windows.zip"
  echo "==> Downloading JDK 17 from Adoptium API..."
  echo "    URL: $JDK17_URL"
  wget -q --show-progress "$JDK17_URL" -O "$tmp_zip"

  echo "==> Extracting to $JDK17_INSTALL_DIR..."
  rm -rf "$JDK17_INSTALL_DIR"
  mkdir -p "$(dirname "$JDK17_INSTALL_DIR")"
  unzip -q "$tmp_zip" -d "/tmp/jdk17-extract"
  local subdir
  subdir="$(find "/tmp/jdk17-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
  mv "$subdir" "$JDK17_INSTALL_DIR"
  rm -rf "/tmp/jdk17-extract" "$tmp_zip"

  echo "✅  JDK 17 installed to: $JDK17_INSTALL_DIR"
  echo "    java version: $("$JDK17_INSTALL_DIR/bin/java.exe" -version 2>&1 | head -n1)"
  SDKMANAGER_JAVA_HOME="$JDK17_INSTALL_DIR"
}

# ────────────────────────────────────────────────────────────────────────────
# Step 3 — Install Android NDK r27d
# ────────────────────────────────────────────────────────────────────────────

install_ndk() {
  echo ""
  echo "═══ Step 3/12 — Installing Android NDK ${NDK_VERSION} ═══"

  if [[ -d "$NDK_INSTALL_DIR/build/cmake" ]] && [[ -f "$NDK_INSTALL_DIR/build/cmake/android.toolchain.cmake" ]]; then
    echo "✅  NDK already installed at: $NDK_INSTALL_DIR"
    return
  fi

  if ! confirm "Download and install Android NDK ${NDK_VERSION} to $NDK_INSTALL_DIR?"; then
    echo "  Skipped."
    return
  fi

  local tmp_zip="/tmp/android-ndk-${NDK_VERSION}-windows.zip"
  echo "==> Downloading Android NDK ${NDK_VERSION}..."
  echo "    URL: $NDK_URL"
  echo "    (This is ~1.5 GB, may take a while...)"
  wget -q --show-progress "$NDK_URL" -O "$tmp_zip"

  echo "==> Extracting NDK..."
  rm -rf "$NDK_INSTALL_DIR"
  mkdir -p "$(dirname "$NDK_INSTALL_DIR")"
  unzip -q "$tmp_zip" -d "/tmp/ndk-extract"
  local subdir
  subdir="$(find "/tmp/ndk-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
  mv "$subdir" "$NDK_INSTALL_DIR"
  rm -rf "/tmp/ndk-extract" "$tmp_zip"

  echo "✅  NDK installed to: $NDK_INSTALL_DIR"
  # Verify the toolchain
  local toolchain="$NDK_INSTALL_DIR/toolchains/llvm/prebuilt/windows-x86_64"
  if [[ -d "$toolchain" ]]; then
    echo "    Toolchain: $toolchain"
    local cc="$toolchain/bin/aarch64-linux-android30-clang.cmd"
    if [[ -f "$cc" ]]; then
      echo "    Cross-compiler: $($cc --version 2>&1 | head -n1)"
    fi
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 5 — Cross-compile SDL2 for Android arm64-v8a
# ────────────────────────────────────────────────────────────────────────────

install_sdl2_android() {
  echo ""
  echo "═══ Step 5/12 — Cross-compiling SDL2 for Android arm64-v8a ═══"

  local sdl2_lib="$ANDROID_DEPS_PATH/lib/libSDL2.so"
  local sdl2_src="$(pwd)/build/SDL-${SDL2_VERSION}"
  local sdl2_build="$(pwd)/build/SDL-${SDL2_VERSION}-android"

  # Check if already built
  if [[ -f "$sdl2_lib" ]]; then
    echo "  ✅  SDL2 already cross-compiled for Android:"
    echo "      $sdl2_lib"
    echo "      $(${sdl2_lib%lib/libSDL2.so}bin/sdl2-config --version 2>/dev/null || echo "version unknown") "
    return
  fi

  # Verify NDK is installed (we need the toolchain file)
  local tc_file="$NDK_INSTALL_DIR/build/cmake/android.toolchain.cmake"
  if [[ ! -f "$tc_file" ]]; then
    echo "❌  NDK toolchain file not found at: $tc_file"
    echo "   Run the NDK installation step first."
    exit 1
  fi

  if ! confirm "Cross-compile SDL2 ${SDL2_VERSION} for Android arm64-v8a? (This may take 5-15 minutes)"; then
    echo "  Skipped — you'll need to build it manually before running build_libmain."
    return
  fi

  # --- Download SDL2 source if needed ---
  if [[ ! -d "$sdl2_src" ]]; then
    local tmp_zip="/tmp/sdl2-${SDL2_VERSION}.zip"
    echo "==> Downloading SDL2 ${SDL2_VERSION} source..."
    echo "    URL: $SDL2_URL"
    wget -q --show-progress "$SDL2_URL" -O "$tmp_zip"

    echo "==> Extracting to $(dirname "$sdl2_src")..."
    mkdir -p "$(dirname "$sdl2_src")"
    unzip -q "$tmp_zip" -d "/tmp/sdl2-extract"
    local subdir
    subdir="$(find "/tmp/sdl2-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
    mv "$subdir" "$sdl2_src"
    rm -rf "/tmp/sdl2-extract" "$tmp_zip"
    echo "    Source extracted to: $sdl2_src"
  else
    echo "    SDL2 source already present: $sdl2_src"
  fi

  # --- Create build directory ---
  mkdir -p "$sdl2_build"

  # --- Determine the parallel jobs count ---
  local jobs
  jobs="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

  # --- Configure with cmake + NDK toolchain ---
  echo "==> Configuring SDL2 for arm64-v8a / ${SDK_PLATFORM}..."
  (cd "$sdl2_build" && cmake -G "Unix Makefiles" "$sdl2_src" -Wno-dev \
    -DCMAKE_TOOLCHAIN_FILE="$tc_file" \
    -DANDROID_ABI="arm64-v8a" \
    -DANDROID_PLATFORM="${SDK_PLATFORM}" \
    -DCMAKE_INSTALL_PREFIX="$ANDROID_DEPS_PATH" \
    -DSDL_ANDROID_PACKAGE_NAME=org.ikemen_engine.ikemen_go \
    -DSDL_STATIC=OFF \
    -DSDL_SHARED=ON \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_SHARED_LINKER_FLAGS="-Wl,-z,max-page-size=16384") || {
      echo "❌  CMake configuration for SDL2 Android failed."
      echo "   Check the error above and ensure NDK is correctly installed."
      exit 1
    }

  # --- Build ---
  echo "==> Building SDL2 for Android (${jobs} parallel jobs)..."
  cmake --build "$sdl2_build" -- -j"$jobs" || {
    echo "❌  SDL2 Android build failed."
    exit 1
  }

  # --- Install ---
  echo "==> Installing SDL2 to $ANDROID_DEPS_PATH..."
  cmake --build "$sdl2_build" --target install || {
    echo "❌  SDL2 Android install failed."
    exit 1
  }

  # --- Verify ---
  if [[ -f "$sdl2_lib" ]]; then
    echo "✅  SDL2 cross-compiled for Android arm64-v8a:"
    echo "      Library: $sdl2_lib"
    ls -lh "$sdl2_lib" 2>/dev/null | awk '{print "      Size: " $5}'
  else
    echo "❌  libSDL2.so not found after install at: $sdl2_lib"
    exit 1
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 6 — Cross-compile libxmp for Android arm64-v8a
# ────────────────────────────────────────────────────────────────────────────

install_libxmp_android() {
  echo ""
  echo "═══ Step 6/12 — Cross-compiling libxmp for Android arm64-v8a ═══"

  local xmp_lib="$ANDROID_DEPS_PATH/lib/libxmp.so"
  local xmp_src="$(pwd)/build/libxmp-${XMP_VERSION}"
  local xmp_build="$(pwd)/build/libxmp-${XMP_VERSION}-android"

  # Check if already built
  if [[ -f "$xmp_lib" ]]; then
    echo "  ✅  libxmp already cross-compiled for Android:"
    ls -lh "$xmp_lib" 2>/dev/null | awk '{print "      Size: " $5", Path: " $NF}'
    return
  fi

  # Verify NDK is installed
  local tc_file="$NDK_INSTALL_DIR/build/cmake/android.toolchain.cmake"
  if [[ ! -f "$tc_file" ]]; then
    echo "❌  NDK toolchain file not found at: $tc_file"
    exit 1
  fi

  if ! confirm "Cross-compile libxmp ${XMP_VERSION} for Android arm64-v8a? (Quick build)"; then
    echo "  Skipped — you'll need to build it manually."
    return
  fi

  # --- Download source ---
  if [[ ! -d "$xmp_src" ]]; then
    local tmp_zip="/tmp/libxmp-${XMP_VERSION}.zip"
    echo "==> Downloading libxmp ${XMP_VERSION} source..."
    wget -q --show-progress "$XMP_URL" -O "$tmp_zip"

    echo "==> Extracting..."
    mkdir -p "$(dirname "$xmp_src")"
    unzip -q "$tmp_zip" -d "/tmp/libxmp-extract"
    local subdir
    subdir="$(find "/tmp/libxmp-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
    mv "$subdir" "$xmp_src"
    rm -rf "/tmp/libxmp-extract" "$tmp_zip"
  else
    echo "    libxmp source already present: $xmp_src"
  fi

  mkdir -p "$xmp_build"
  local jobs
  jobs="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

  echo "==> Configuring libxmp for arm64-v8a / ${SDK_PLATFORM}..."
  (cd "$xmp_build" && cmake "$xmp_src" -Wno-dev \
    -Wno-deprecated \
    -DCMAKE_TOOLCHAIN_FILE="$tc_file" \
    -DANDROID_ABI="arm64-v8a" \
    -DANDROID_PLATFORM="${SDK_PLATFORM}" \
    -DCMAKE_INSTALL_PREFIX="$ANDROID_DEPS_PATH" \
    -DBUILD_STATIC=OFF \
    -DBUILD_SHARED=ON \
    -DCMAKE_SHARED_LINKER_FLAGS="-Wl,-z,max-page-size=16384") || {
      echo "❌  CMake configuration for libxmp Android failed."
      exit 1
    }

  echo "==> Building libxmp for Android (${jobs} jobs)..."
  cmake --build "$xmp_build" -- -j"$jobs" || {
    echo "❌  libxmp Android build failed."
    exit 1
  }

  echo "==> Installing libxmp to $ANDROID_DEPS_PATH..."
  cmake --build "$xmp_build" --target install || {
    echo "❌  libxmp Android install failed."
    exit 1
  }

  if [[ -f "$xmp_lib" ]]; then
    echo "✅  libxmp cross-compiled for Android:"
    ls -lh "$xmp_lib" 2>/dev/null | awk '{print "      Size: " $5}'
  else
    echo "❌  libxmp.so not found after install at: $xmp_lib"
    exit 1
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 7 — Cross-compile FFmpeg for Android arm64-v8a
# ────────────────────────────────────────────────────────────────────────────

install_ffmpeg_android() {
  echo ""
  echo "═══ Step 7/12 — Cross-compiling FFmpeg for Android arm64-v8a ═══"

  local ffmpeg_pc="$ANDROID_DEPS_PATH/lib/pkgconfig/libavformat.pc"
  local ffmpeg_src="$(pwd)/build/FFmpeg-${FFMPEG_VERSION}"

  # Check if already built
  if [[ -f "$ffmpeg_pc" ]]; then
    echo "  ✅  FFmpeg already cross-compiled for Android:"
    echo "      pkgconfig: $ffmpeg_pc"
    return
  fi

  # Verify NDK toolchain
  local toolchain="$NDK_INSTALL_DIR/toolchains/llvm/prebuilt/windows-x86_64"
  if [[ ! -d "$toolchain" ]]; then
    echo "❌  NDK toolchain not found at: $toolchain"
    exit 1
  fi

  if ! confirm "Cross-compile FFmpeg ${FFMPEG_VERSION} for Android arm64-v8a? (This may take 15-30 minutes)"; then
    echo "  Skipped — you'll need to build it manually."
    return
  fi

  # --- Download source ---
  if [[ ! -d "$ffmpeg_src" ]]; then
    local tmp_zip="/tmp/ffmpeg-${FFMPEG_VERSION}.zip"
    echo "==> Downloading FFmpeg ${FFMPEG_VERSION} source..."
    echo "    URL: $FFMPEG_URL"
    wget -q --show-progress "$FFMPEG_URL" -O "$tmp_zip"

    echo "==> Extracting..."
    mkdir -p "$(dirname "$ffmpeg_src")"
    unzip -q "$tmp_zip" -d "/tmp/ffmpeg-extract"
    local subdir
    subdir="$(find "/tmp/ffmpeg-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
    mv "$subdir" "$ffmpeg_src"
    rm -rf "/tmp/ffmpeg-extract" "$tmp_zip"
  else
    echo "    FFmpeg source already present: $ffmpeg_src"
  fi

  # Clean stale config.h from any previous (native) build.
  # FFmpeg refuses out-of-tree configure when config.h exists in the source.
  if [[ -f "$ffmpeg_src/config.h" ]]; then
    echo "==> Cleaning previously configured FFmpeg source..."
    (cd "$ffmpeg_src" && make distclean 2>/dev/null || true)
    rm -f "$ffmpeg_src/config.h" 2>/dev/null || true
  fi

  # Setup NDK cross-compiler paths
  local arch="aarch64"
  local api="$ANDROID_API"
  local cc_compiler="$toolchain/bin/${arch}-linux-android${api}-clang"
  local ar_tool="$toolchain/bin/llvm-ar"
  local nm_tool="$toolchain/bin/llvm-nm"
  local strip_tool="$toolchain/bin/llvm-strip"
  local jobs
  jobs="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

  echo "==> Configuring FFmpeg for aarch64 / ${SDK_PLATFORM} (in-tree)..."
  # FFmpeg uses autotools; configure + build in-tree (inside source dir)
  (cd "$ffmpeg_src" && ./configure \
    --prefix="$ANDROID_DEPS_PATH" \
    --enable-cross-compile \
    --target-os=android \
    --arch=$arch \
    --cc="$cc_compiler" \
    --ar="$ar_tool" \
    --nm="$nm_tool" \
    --strip="$strip_tool" \
    --extra-cflags="-fPIC" \
    --extra-ldflags="-Wl,-z,max-page-size=16384" \
    --enable-shared --disable-static \
    --disable-gpl --disable-nonfree \
    --disable-debug --disable-doc --disable-programs --disable-everything \
    --disable-autodetect \
    --enable-avformat --enable-avcodec --enable-avutil \
    --enable-swresample --enable-swscale \
    --enable-avfilter --enable-filter=buffer,buffersink,format,scale,pad,crop \
    --enable-protocol=file \
    --enable-demuxer=matroska,webm \
    --enable-decoder=vp8,vp9,opus,vorbis \
    --enable-parser=vp8,vp9,opus,vorbis \
    --enable-jni --enable-mediacodec \
    --pkg-config="$(which pkg-config)") || {
      echo "❌  FFmpeg configure failed. Check the error above."
      exit 1
    }

  echo "==> Building FFmpeg for Android (${jobs} parallel jobs)..."
  (cd "$ffmpeg_src" && make -j"$jobs") || {
    echo "❌  FFmpeg Android build failed."
    exit 1
  }

  echo "==> Installing FFmpeg to $ANDROID_DEPS_PATH..."
  (cd "$ffmpeg_src" && make install) || {
    echo "❌  FFmpeg Android install failed."
    exit 1
  }

  # Create dummy gl.pc for pkg-config (needed by Go build)
  create_dummy_gl_pc

  if [[ -f "$ffmpeg_pc" ]]; then
    echo "✅  FFmpeg cross-compiled for Android:"
    echo "      pkgconfig: $ffmpeg_pc"
    ls -lh "$ANDROID_DEPS_PATH/lib/libavformat.so" 2>/dev/null | awk '{print "      Size: " $5}'
  else
    echo "❌  FFmpeg pkg-config files not found at: $(dirname "$ffmpeg_pc")"
    exit 1
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Helper — create dummy gl.pc for Android pkg-config
# ────────────────────────────────────────────────────────────────────────────

create_dummy_gl_pc() {
  local pc_dir="$ANDROID_DEPS_PATH/lib/pkgconfig"
  local gl_pc="$pc_dir/gl.pc"
  if [[ ! -f "$gl_pc" ]]; then
    mkdir -p "$pc_dir"
    cat > "$gl_pc" <<-EOF
	Name: gl
	Description: Android GLES fake
	Version: 1.0
	Libs:
	Cflags:
	EOF
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 8 — Install Android SDK cmdline-tools + platform-30 + build-tools
# ────────────────────────────────────────────────────────────────────────────

install_sdk() {
  echo ""
  echo "═══ Step 8/12 — Installing Android SDK (${SDK_PLATFORM} + build-tools ${SDK_BUILD_TOOLS}) ═══"

  if [[ -d "$SDK_INSTALL_DIR/platforms/$SDK_PLATFORM" ]] && \
     [[ -d "$SDK_INSTALL_DIR/build-tools/$SDK_BUILD_TOOLS" ]]; then
    echo "✅  Android SDK already installed at: $SDK_INSTALL_DIR"
    return
  fi

  if ! confirm "Download and install Android SDK at $SDK_INSTALL_DIR?"; then
    echo "  Skipped."
    return
  fi

  # --- Ensure a JDK 17+ is on PATH for sdkmanager ---
  # The latest cmdline-tools (build 11076708+) require Java 17+.
  if [[ -n "${SDKMANAGER_JAVA_HOME:-}" ]] && [[ -x "$SDKMANAGER_JAVA_HOME/bin/java" ]]; then
    export JAVA_HOME="$SDKMANAGER_JAVA_HOME"
    export PATH="$SDKMANAGER_JAVA_HOME/bin:$PATH"
    echo "    Using JDK 17 (sdkmanager): $("$SDKMANAGER_JAVA_HOME/bin/java" -version 2>&1 | head -n1)"
  elif [[ -n "${JAVA_HOME:-}" ]] && command -v java &>/dev/null; then
    local jmajor
    jmajor="$(java_major_version "$(command -v java)")"
    echo "    Using existing JAVA_HOME: $(java -version 2>&1 | head -n1)"
    if [[ "$jmajor" -lt 17 ]]; then
      echo "    ⚠️  Java version $jmajor is < 17 — sdkmanager may fail!"
    fi
  elif [[ -x "$JDK17_INSTALL_DIR/bin/java" ]]; then
    export JAVA_HOME="$JDK17_INSTALL_DIR"
    export PATH="$JDK17_INSTALL_DIR/bin:$PATH"
    echo "    Using JDK 17 (fallback): $("$JDK17_INSTALL_DIR/bin/java" -version 2>&1 | head -n1)"
  else
    echo "ERROR: No JDK found. Install JDK 17 first or set JAVA_HOME."
    exit 1
  fi
  # Verify java is available
  if ! java -version &>/dev/null; then
    echo "ERROR: 'java' command not found even though JAVA_HOME=$JAVA_HOME"
    exit 1
  fi

  # --- Install cmdline-tools (needed for sdkmanager) ---
  local sdkmanager="$SDK_INSTALL_DIR/cmdline-tools/latest/bin/sdkmanager.bat"
  if [[ ! -f "$sdkmanager" ]]; then
    local tmp_zip="/tmp/cmdline-tools-win.zip"
    echo "==> Downloading Android SDK cmdline-tools..."
    echo "    URL: $CMDLINE_TOOLS_URL"
    wget -q --show-progress "$CMDLINE_TOOLS_URL" -O "$tmp_zip"

    echo "==> Extracting cmdline-tools..."
    mkdir -p "/tmp/cmdline-tools-extract"
    unzip -q "$tmp_zip" -d "/tmp/cmdline-tools-extract"
    # The zip contains a top-level "cmdline-tools" dir; move to "latest"
    rm -rf "$SDK_INSTALL_DIR/cmdline-tools/latest"
    mkdir -p "$SDK_INSTALL_DIR/cmdline-tools"
    if [[ -d "/tmp/cmdline-tools-extract/cmdline-tools" ]]; then
      mv "/tmp/cmdline-tools-extract/cmdline-tools" "$SDK_INSTALL_DIR/cmdline-tools/latest"
    else
      local subdir
      subdir="$(find "/tmp/cmdline-tools-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
      mv "$subdir" "$SDK_INSTALL_DIR/cmdline-tools/latest"
    fi
    rm -rf "/tmp/cmdline-tools-extract" "$tmp_zip"
    echo "    cmdline-tools installed."
  else
    echo "    cmdline-tools already present."
  fi

  # --- Accept licenses ---
  echo "==> Accepting SDK licenses..."
  yes | "$sdkmanager" --sdk_root="$SDK_INSTALL_DIR" --licenses 2>/dev/null || true

  # --- Install platform, build-tools, and platform-tools ---
  echo "==> Installing $SDK_PLATFORM..."
  "$sdkmanager" --sdk_root="$SDK_INSTALL_DIR" "platforms;$SDK_PLATFORM"

  echo "==> Installing build-tools $SDK_BUILD_TOOLS..."
  "$sdkmanager" --sdk_root="$SDK_INSTALL_DIR" "build-tools;$SDK_BUILD_TOOLS"

  echo "==> Installing platform-tools (adb)..."
  "$sdkmanager" --sdk_root="$SDK_INSTALL_DIR" "platform-tools"

  echo "✅  Android SDK ready at: $SDK_INSTALL_DIR"
  echo "    Platform:  $SDK_PLATFORM"
  echo "    Build-tools: $SDK_BUILD_TOOLS"
  if [[ -d "$SDK_INSTALL_DIR/platform-tools" ]]; then
    echo "    platform-tools: present (adb)"
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 5 — Set up environment variables
# ────────────────────────────────────────────────────────────────────────────

setup_env() {
  echo ""
  echo "═══ Step 9/12 — Setting up environment variables ═══"

  local bashrc="$HOME/.bashrc"
  local marker="# >>> Ikemen-GO Android 11 toolchain >>>"
  local end_marker="# <<< Ikemen-GO Android 11 toolchain <<<"

  # Ensure .bashrc exists
  touch "$bashrc"

  # Check if already configured
  if grep -q "$marker" "$bashrc" 2>/dev/null; then
    echo "⚠️  Environment block already exists in $bashrc"
    if ! confirm "Overwrite it?"; then
      echo "  Skipped."
      return
    fi
    # Remove old block
    sed -i "/$marker/,/$end_marker/d" "$bashrc"
  fi

  cat >> "$bashrc" <<-EOF

	$marker
	# Android NDK (for cross-compilation)
	export ANDROID_NDK_HOME="$NDK_INSTALL_DIR"
	# Android SDK (for APK packaging)
	export ANDROID_SDK_ROOT="$SDK_INSTALL_DIR"
	export ANDROID_HOME="$SDK_INSTALL_DIR"
	# Prebuilt SDL2 android deps path (for android target)
	export ANDROID_DEPS_PATH="$(cygpath -m "$ANDROID_DEPS_PATH")"
	# JDK 17 (default JAVA_HOME)
	export JAVA_HOME="$JDK17_INSTALL_DIR"
	# Prepend to PATH
	export PATH="\$JAVA_HOME/bin:\$ANDROID_SDK_ROOT/cmdline-tools/latest/bin:\$ANDROID_SDK_ROOT/platform-tools:\$PATH"
	$end_marker
	EOF

  echo "✅  Environment variables written to: $bashrc"
  echo ""
  echo "    Key variables:"
  echo "      ANDROID_NDK_HOME    = $NDK_INSTALL_DIR"
  echo "      ANDROID_SDK_ROOT    = $SDK_INSTALL_DIR"
  echo "      ANDROID_DEPS_PATH   = $ANDROID_DEPS_PATH"
  echo "      JAVA_HOME           = $JDK17_INSTALL_DIR"
  echo ""
  echo "    Run 'source ~/.bashrc' or restart your shell to apply."
}

# ────────────────────────────────────────────────────────────────────────────
# Step 10 — Installation summary & verification
# ────────────────────────────────────────────────────────────────────────────

verify_installation() {
  echo ""
  echo "╔══════════════════════════════════════════════════════════╗"
  echo "║                 Installation Summary                    ║"
  echo "╚══════════════════════════════════════════════════════════╝"

  local all_ok=true

  echo ""
  echo "── Build tools ──"
  for tool in make cmake gcc g++ nasm pkg-config go wget unzip; do
    if command -v "$tool" &>/dev/null; then
      echo "  ✅  $tool"
    else
      echo "  ❌  $tool — NOT FOUND"
      all_ok=false
    fi
  done

  echo ""
  echo "── Go ──"
  if command -v go &>/dev/null; then
    echo "  ✅  $(go version)"
  else
    echo "  ❌  Go not on PATH"
    all_ok=false
  fi

  echo ""
  echo "── JDK 17 ──"
  if [[ -f "$JDK17_INSTALL_DIR/bin/java.exe" ]]; then
    local jver
    jver="$("$JDK17_INSTALL_DIR/bin/java.exe" -version 2>&1 | head -n1)"
    echo "  ✅  $jver"
    echo "      Path: $JDK17_INSTALL_DIR"
  else
    echo "  ❌  JDK 17 not found at $JDK17_INSTALL_DIR"
    all_ok=false
  fi

  echo ""
  echo "── Android NDK ──"
  local tc="$NDK_INSTALL_DIR/toolchains/llvm/prebuilt/windows-x86_64"
  if [[ -d "$tc" ]]; then
    echo "  ✅  NDK $NDK_VERSION"
    echo "      Path: $NDK_INSTALL_DIR"
    local cc="$tc/bin/aarch64-linux-android30-clang.cmd"
    if [[ -f "$cc" ]]; then
      echo "      Cross-compiler: present"
    else
      echo "      ⚠️  Cross-compiler not found (aarch64-linux-android30-clang)"
    fi
  else
    echo "  ❌  NDK toolchain not found at $tc"
    all_ok=false
  fi

  echo ""
  echo "── SDL2 Android deps ──"
  if [[ -f "$ANDROID_DEPS_PATH/lib/libSDL2.so" ]]; then
    echo "  ✅  SDL2 cross-compiled for arm64-v8a"
    local sdl2_size
    sdl2_size="$(ls -lh "$ANDROID_DEPS_PATH/lib/libSDL2.so" 2>/dev/null | awk '{print $5}')"
    echo "      Size: ${sdl2_size:-unknown}, Path: $ANDROID_DEPS_PATH"
  else
    echo "  ❌  SDL2 Android deps not found at: $ANDROID_DEPS_PATH"
    echo "      Build them with this script (steps 5-7) or re-run."
    all_ok=false
  fi

  echo ""
  echo "── libxmp Android deps ──"
  local xmp_pc="$ANDROID_DEPS_PATH/lib/pkgconfig/libxmp.pc"
  if [[ -f "$xmp_pc" ]]; then
    echo "  ✅  libxmp cross-compiled for arm64-v8a"
    echo "      pkgconfig: $xmp_pc"
  else
    echo "  ❌  libxmp not found at: $ANDROID_DEPS_PATH"
    all_ok=false
  fi

  echo ""
  echo "── FFmpeg Android deps ──"
  local ffmpeg_pc="$ANDROID_DEPS_PATH/lib/pkgconfig/libavformat.pc"
  if [[ -f "$ffmpeg_pc" ]]; then
    echo "  ✅  FFmpeg cross-compiled for arm64-v8a"
    echo "      pkgconfig: $ffmpeg_pc"
  else
    echo "  ❌  FFmpeg not found at: $ANDROID_DEPS_PATH"
    all_ok=false
  fi

  echo ""
  echo "── Android SDK ──"
  if [[ -d "$SDK_INSTALL_DIR/platforms/$SDK_PLATFORM" ]]; then
    echo "  ✅  Platform $SDK_PLATFORM"
  else
    echo "  ❌  Platform $SDK_PLATFORM not found"
    all_ok=false
  fi
  if [[ -d "$SDK_INSTALL_DIR/build-tools/$SDK_BUILD_TOOLS" ]]; then
    echo "  ✅  Build-tools $SDK_BUILD_TOOLS"
  else
    echo "  ❌  Build-tools $SDK_BUILD_TOOLS not found"
    all_ok=false
  fi
  if [[ -f "$SDK_INSTALL_DIR/cmdline-tools/latest/bin/sdkmanager.bat" ]]; then
    echo "  ✅  sdkmanager"
  else
    echo "  ❌  sdkmanager not found"
    all_ok=false
  fi
  echo "      Path: $SDK_INSTALL_DIR"

  echo ""
  if $all_ok; then
    echo "🎉  All tools installed successfully!"
    echo ""
    echo "  The script will now build libmain.so and the APK automatically."
    echo "  Or run steps manually:"
    echo "    source ~/.bashrc"
    echo "    ./tools/generate_android_via_native.sh --yes"
  else
    echo "⚠️  Some tools are missing or incomplete (see ❌ above)."
    echo "   Check the output above and re-run the script if needed."
    exit 1
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Step 10 — Build libmain.so (Android arm64 shared library via NDK + Go)
# ────────────────────────────────────────────────────────────────────────────

build_libmain() {
  echo ""
  echo "═══ Step 11/12 — Building libmain.so (arm64-v8a) ═══"

  # --- Validate NDK ---
  local toolchain="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/windows-x86_64"
  if [[ ! -d "$toolchain" ]]; then
    echo "❌  NDK toolchain not found: $toolchain"
    echo "   Set ANDROID_NDK_HOME or install NDK first."
    exit 1
  fi

  # --- Validate android-deps (SDL2) ---
  if [[ ! -d "$ANDROID_DEPS_PATH/lib" ]]; then
    echo "❌  SDL2 android deps not found: $ANDROID_DEPS_PATH"
    echo "   Run steps 5-7 first to cross-compile SDL2/libxmp/FFmpeg."
    exit 1
  fi

  # --- Derive compiler paths ---
  local android_target="aarch64-linux-android${ANDROID_API}"
  local cc="$toolchain/bin/${android_target}-clang"
  local cxx="$toolchain/bin/${android_target}-clang++"
  if [[ ! -x "$cc" ]]; then
    echo "❌  Cross-compiler not found: $cc"
    exit 1
  fi

  # --- Convert paths for Windows CGo ---
  local deps_include="$ANDROID_DEPS_PATH/include"
  local deps_lib="$ANDROID_DEPS_PATH/lib"
  if command -v cygpath &>/dev/null; then
    deps_include="$(cygpath -m "$deps_include")"
    deps_lib="$(cygpath -m "$deps_lib")"
  fi

  # --- Build ---
  mkdir -p "$(dirname "$ANDROID_BINARY")"
  echo "  GOOS=android GOARCH=arm64 CC=$cc"
  echo "  Output: $ANDROID_BINARY"

  CGO_ENABLED=1 GOOS=android GOARCH=arm64 GOEXPERIMENT=arenas \
  CC="$cc" CXX="$cxx" \
  PKG_CONFIG_LIBDIR="$ANDROID_DEPS_PATH/lib/pkgconfig" \
  PKG_CONFIG_SYSROOT_DIR="$ANDROID_DEPS_PATH" \
  PKG_CONFIG_PATH= \
  CGO_CFLAGS="-I$deps_include -I$deps_include/SDL2" \
  CGO_LDFLAGS="-L$deps_lib -lSDL2 -lGLESv2 -lOpenSLES -llog -Wl,-z,max-page-size=16384" \
  go build -buildmode=c-shared -trimpath -v -tags "mugen lite android gles2" \
    -ldflags "-s -w -X 'main.Version=nightly' -X 'runtime.godebugDefault=asyncpreemptoff=1,sigaltstack=0'" \
    -o "$ANDROID_BINARY" ./src

  if [[ ! -f "$ANDROID_BINARY" ]]; then
    echo "❌  Build failed: $ANDROID_BINARY not found"
    exit 1
  fi
  echo "✅  libmain.so built: $ANDROID_BINARY"
}

# ────────────────────────────────────────────────────────────────────────────
# Step 12 — Download ikemen-droid source (for APK build)
# ────────────────────────────────────────────────────────────────────────────

download_ikemen_droid_source() {
  echo ""
  echo "═══ Step 12/12 — Downloading ikemen-droid source ═══"

  # If source already exists, skip
  if [[ -d "$IKEMEN_DROID_SRC" ]]; then
    echo "  ✅  ikemen-droid source already present at: $IKEMEN_DROID_SRC"
    return
  fi

  if ! confirm "Download ikemen-droid source from $IKEMEN_DROID_URL to $IKEMEN_DROID_SRC?"; then
    echo "  Skipped — you'll need to provide the source manually before the APK build."
    return
  fi

  local tmp_zip="/tmp/ikemen-droid.zip"
  echo "==> Downloading ikemen-droid source..."
  echo "    URL: $IKEMEN_DROID_URL"
  wget -q --show-progress "$IKEMEN_DROID_URL" -O "$tmp_zip"

  echo "==> Extracting to $IKEMEN_DROID_SRC..."
  mkdir -p "$(dirname "$IKEMEN_DROID_SRC")"
  unzip -q "$tmp_zip" -d "/tmp/ikemen-droid-extract"
  local subdir
  subdir="$(find "/tmp/ikemen-droid-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
  mv "$subdir" "$IKEMEN_DROID_SRC"
  rm -rf "/tmp/ikemen-droid-extract" "$tmp_zip"

  echo "✅  ikemen-droid source downloaded to: $IKEMEN_DROID_SRC"
}

# ────────────────────────────────────────────────────────────────────────────
# Step 13 — Download screenpack assets (for APK runtime)
# ────────────────────────────────────────────────────────────────────────────

download_screenpack() {
  echo ""
  echo "═══ Step 13/12 — Downloading screenpack assets ═══"

  local screenpack_url="https://github.com/leonkasovan/Ikemen-GO-Screenpack/archive/refs/heads/master.zip"
  local screenpack_dir="$(pwd)/deploy"
  local marker_file="$screenpack_dir/.screenpack_done"

  # If already downloaded, skip
  if [[ -f "$marker_file" ]]; then
    echo "  ✅  Screenpack already present at: $screenpack_dir"
    return
  fi

  if ! confirm "Download screenpack from $screenpack_url to $screenpack_dir?"; then
    echo "  Skipped — APK may miss runtime assets (chars, stages, sounds, etc.)."
    return
  fi

  local tmp_zip="/tmp/screenpack.zip"
  echo "==> Downloading screenpack..."
  echo "    URL: $screenpack_url"
  wget -q --show-progress "$screenpack_url" -O "$tmp_zip"

  echo "==> Extracting to $screenpack_dir..."
  mkdir -p "$screenpack_dir"
  unzip -q "$tmp_zip" -d "/tmp/screenpack-extract"
  local subdir
  subdir="$(find "/tmp/screenpack-extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
  # Move contents into deploy/ (overwrite/merge)
  cp -a "$subdir"/. "$screenpack_dir/"
  rm -rf "/tmp/screenpack-extract" "$tmp_zip"

  # Mark as done
  touch "$marker_file"
  echo "✅  Screenpack downloaded to: $screenpack_dir"
}

# ────────────────────────────────────────────────────────────────────────────
# Helper — ensure an Android signing keystore exists (auto-generate if missing)
# ────────────────────────────────────────────────────────────────────────────

ensure_android_keystore() {
  # Keystore explicitly disabled?
  if [[ -z "$ANDROID_KEYSTORE" ]]; then
    echo "  ℹ️  ANDROID_KEYSTORE not set — APK will be left unsigned."
    return 1
  fi

  # Already exists?
  if [[ -f "$ANDROID_KEYSTORE" ]]; then
    echo "  ✅  Keystore already present: $ANDROID_KEYSTORE"
    return 0
  fi

  # Locate keytool (JDK 17 is on PATH by this point in build_apk)
  local kt
  kt="$(command -v keytool 2>/dev/null || true)"
  [[ -z "$kt" && -x "$JAVA_HOME/bin/keytool" ]] && kt="$JAVA_HOME/bin/keytool"
  if [[ -z "$kt" ]]; then
    echo "  ⚠️  keytool not found — cannot auto-generate keystore; APK will be unsigned."
    return 1
  fi

  # apksigner uses the 'pass:' prefix; keytool needs the raw password.
  local store_pass key_pass
  case "$ANDROID_KEYSTORE_PASS" in
    pass:*) store_pass="${ANDROID_KEYSTORE_PASS#pass:}" ;;
    *)      store_pass="$ANDROID_KEYSTORE_PASS" ;;
  esac
  case "$ANDROID_KEY_PASS" in
    pass:*) key_pass="${ANDROID_KEY_PASS#pass:}" ;;
    *)      key_pass="$ANDROID_KEY_PASS" ;;
  esac

  echo "==> Generating Android release keystore: $ANDROID_KEYSTORE"
  mkdir -p "$(dirname "$ANDROID_KEYSTORE")"
  "$kt" -genkeypair -v \
    -keystore "$ANDROID_KEYSTORE" \
    -alias "$ANDROID_KEY_ALIAS" \
    -keyalg RSA -keysize 2048 -validity 10000 \
    -storepass "$store_pass" -keypass "$key_pass" \
    -dname "CN=Ikemen GO, OU=Engine, O=Ikemen, L=Unknown, ST=Unknown, C=US" \
    2>&1 | tail -n 2

  if [[ -f "$ANDROID_KEYSTORE" ]]; then
    echo "✅  Keystore generated: $ANDROID_KEYSTORE"
    return 0
  fi
  echo "  ⚠️  Keystore generation failed — APK will be unsigned."
  return 1
}

# ────────────────────────────────────────────────────────────────────────────
# Step 14 — Build APK from local ikemen-droid source
# ────────────────────────────────────────────────────────────────────────────

build_apk() {
  echo ""
  echo "═══ Step 14/12 — Building Android APK ═══"

  # --- Ensure ikemen-droid source is available (download if missing) ---
  if [[ ! -d "$IKEMEN_DROID_SRC" ]]; then
    echo "  ikemen-droid source not found at: $IKEMEN_DROID_SRC"
    download_ikemen_droid_source
  fi

  # --- Validate prerequisites ---
  if [[ ! -d "$IKEMEN_DROID_SRC" ]]; then
    echo "❌  ikemen-droid source not found: $IKEMEN_DROID_SRC"
    echo "   Set IKEMEN_DROID_SRC or IKEMEN_DROID_URL and re-run."
    exit 1
  fi

  local libmain="$ANDROID_BINARY"
  if [[ ! -f "$libmain" ]]; then
    echo "❌  libmain.so not found: $libmain"
    echo "   Run step 10 (build_libmain) first."
    exit 1
  fi

  if [[ ! -d "$ANDROID_DEPS_PATH/lib" ]]; then
    echo "❌  Android SDL2 deps not found: $ANDROID_DEPS_PATH"
    echo "   Run the setup script steps 5-7 first."
    exit 1
  fi

  # --- Verify JAVA_HOME ---
  export JAVA_HOME="$JAVA_HOME"
  export PATH="$JAVA_HOME/bin:$PATH"
  local java_bin="$JAVA_HOME/bin/java"
  if [[ ! -x "$java_bin" ]] && ! command -v java &>/dev/null; then
    echo "❌  java not found. Set JAVA_HOME or install JDK 17."
    exit 1
  fi

  local sdk="${ANDROID_SDK_ROOT:-$ANDROID_HOME}"
  if [[ -z "$sdk" ]] || [[ ! -d "$sdk" ]]; then
    echo "❌  Android SDK not found. Set ANDROID_SDK_ROOT or ANDROID_HOME."
    exit 1
  fi
  echo "  SDK:  $sdk"
  echo "  JDK:  $JAVA_HOME"

  # --- Copy ikemen-droid source to build dir ---
  echo ""
  echo "==> Syncing ikemen-droid..."
  local src_real
  src_real="$(cd "$IKEMEN_DROID_SRC" && pwd -P)"
  local dst_parent
  dst_parent="$(dirname "$IKEMEN_DROID_DIR")"
  local dst_real
  if [[ -d "$dst_parent" ]]; then
    dst_real="$(cd "$dst_parent" && pwd -P)/$(basename "$IKEMEN_DROID_DIR")"
  else
    dst_real="$IKEMEN_DROID_DIR"
  fi

  if [[ "$src_real" != "$dst_real" ]]; then
    rm -rf "$IKEMEN_DROID_DIR"
    mkdir -p "$dst_parent"
    echo "    Copying from: $IKEMEN_DROID_SRC"
    cp -a "$IKEMEN_DROID_SRC" "$IKEMEN_DROID_DIR"
  else
    echo "    Source and dest are the same, skipping copy."
  fi

  # --- AGP version ---
  # Gradle 8.1.1 requires JDK 17+ to run, so AGP 8.1.1 (already in
  # build.gradle) works as-is. No downgrade needed.
  echo ""
  echo "==> Using AGP version from build.gradle:"
  local bf="$IKEMEN_DROID_DIR/build.gradle"
  if [[ -f "$bf" ]]; then
    grep 'com.android.tools.build:gradle' "$bf" | head -1 | sed 's/^/    /'
  fi

  # --- Ensure runtime assets ---
  echo ""
  echo "==> Ensuring runtime assets..."
  if [[ ! -f "external/gamecontrollerdb.txt" ]]; then
    echo "    Downloading gamecontrollerdb.txt..."
    wget -q "https://raw.githubusercontent.com/mdqinc/SDL_GameControllerDB/refs/heads/master/gamecontrollerdb.txt" -O "external/gamecontrollerdb.txt"
  fi
  if [[ ! -f "data/system.base.def" ]]; then
    echo "    Generating data/system.base.def from defaultMotif.ini..."
    mkdir -p data
    cp -a "src/resources/defaultMotif.ini" "data/system.base.def"
  fi

  # --- Generate manifest.txt from actual screenpack files in deploy/ ---
  echo ""
  echo "==> Generating manifest.txt from screenpack in deploy/..."
  local manifest_gen="$IKEMEN_DROID_DIR/app/src/main/assets/manifest.txt"
  local screenpack_root="$(pwd)/deploy"
  if [[ -d "$screenpack_root" ]]; then
    # Create manifest from all files under deploy/ (relative paths)
    (cd "$screenpack_root" && find . -type f ! -name ".screenpack_done" | sed 's|^\./||' | sort) > "$manifest_gen"
    echo "    Generated manifest.txt with $(wc -l < "$manifest_gen") entries"
  else
    echo "    ⚠️  deploy/ not found — keeping original manifest.txt"
  fi

  # --- Stage native libs ---
  local app_dir="$IKEMEN_DROID_DIR/app"
  local abi_dir="$app_dir/src/main/jniLibs/arm64-v8a"
  echo ""
  echo "==> Staging native libs into: $abi_dir"
  mkdir -p "$abi_dir"
  rm -f "$abi_dir"/*.so* 2>/dev/null || true
  cp -av "$libmain" "$abi_dir/"
  cp -av "$ANDROID_DEPS_PATH/lib/"*.so* "$abi_dir/" 2>/dev/null || true

  # --- Stage assets from manifest.txt ---
  local assets_dir="$app_dir/src/main/assets"
  local manifest="$assets_dir/manifest.txt"
  if [[ ! -f "$manifest" ]]; then
    echo "❌  ikemen-droid manifest not found at: $manifest"
    exit 1
  fi
  echo ""
  echo "==> Staging assets into: $assets_dir (from manifest.txt)"
  find "$assets_dir" -mindepth 1 -maxdepth 1 ! -name "manifest.txt" -exec rm -rf {} + 2>/dev/null || true
  while IFS= read -r p; do
    [[ -z "$p" ]] && continue
    local src_path="$(pwd)/$p"
    local dst_path="$assets_dir/$p"
    # Fallback to install dir (screenpack)
    [[ ! -e "$src_path" ]] && [[ -e "$(pwd)/deploy/$p" ]] && src_path="$(pwd)/deploy/$p"
    if [[ -d "$src_path" ]]; then
      mkdir -p "$dst_path"
      cp -a "$src_path/." "$dst_path/" 2>/dev/null || true
    elif [[ -f "$src_path" ]]; then
      mkdir -p "$(dirname "$dst_path")"
      cp -a "$src_path" "$dst_path" 2>/dev/null || true
    else
      echo "    WARNING: asset path missing: $p" >&2
    fi
  done < "$manifest"

  # --- Run Gradle ---
  echo ""
  echo "==> Running Gradle ($ANDROID_GRADLE_TASK)..."
  (
    cd "$IKEMEN_DROID_DIR"
    printf "sdk.dir=%s\n" "$sdk" > local.properties
    # Gradle 8.x requires Java 17+ to run; use JDK 17 for the build
    local gradle_java="${SDKMANAGER_JAVA_HOME:-$JDK17_INSTALL_DIR}"
    if [[ ! -x "$gradle_java/bin/java" ]]; then
      gradle_java="$JDK17_INSTALL_DIR"
    fi
    export JAVA_HOME="$gradle_java"
    export PATH="$gradle_java/bin:$PATH"
    echo "    Using Java: $(java -version 2>&1 | head -n1)"
    chmod +x ./gradlew 2>/dev/null || true
    ./gradlew --no-daemon clean "$ANDROID_GRADLE_TASK"
  )
  local rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "❌  Gradle build failed (exit code $rc)."
    exit $rc
  fi

  # --- Copy APK ---
  local apk_dir="$IKEMEN_DROID_DIR/app/build/outputs/apk/$ANDROID_APK_VARIANT"
  local apk_src="$apk_dir/$ANDROID_APK_ARTIFACT"
  if [[ ! -f "$apk_src" ]]; then
    apk_src="$(ls "$apk_dir"/*.apk 2>/dev/null | head -n1)"
  fi
  if [[ -z "$apk_src" ]] || [[ ! -f "$apk_src" ]]; then
    echo "❌  Gradle finished but no APK found in: $apk_dir"
    exit 1
  fi
  mkdir -p "$(dirname "$APK_OUTPUT")"
  cp -av "$apk_src" "$APK_OUTPUT"

  # --- Ensure signing keystore exists (auto-generate if missing) ---
  ensure_android_keystore

  # --- Sign APK if keystore is provided ---
  if [[ -n "$ANDROID_KEYSTORE" ]] && [[ -f "$ANDROID_KEYSTORE" ]]; then
    echo ""
    echo "==> Signing APK..."
    local sdk_bt=""
    sdk_bt="$(ls -d "$sdk"/build-tools/*/ 2>/dev/null | sort -V | tail -n1)"
    sdk_bt="${sdk_bt%/}"
    if [[ -z "$sdk_bt" ]] || [[ ! -d "$sdk_bt" ]]; then
      echo "❌  Android build-tools not found in: $sdk"
      echo "   Install via sdkmanager or set ANDROID_BUILD_TOOLS."
      exit 1
    fi
    local zipalign="$sdk_bt/zipalign"
    [[ ! -f "$zipalign" ]] && zipalign="$sdk_bt/zipalign.exe"
    local apksigner="$sdk_bt/apksigner"
    [[ ! -f "$apksigner" ]] && apksigner="$sdk_bt/apksigner.bat"
    if [[ ! -f "$zipalign" ]]; then
      echo "❌  zipalign not found in: $sdk_bt"
      exit 1
    fi
    if [[ ! -f "$apksigner" ]]; then
      echo "❌  apksigner not found in: $sdk_bt"
      exit 1
    fi

    # zipalign
    local aligned="${APK_OUTPUT}.aligned"
    "$zipalign" -f -p 4 "$APK_OUTPUT" "$aligned"

    # sign
    "$apksigner" sign \
      --ks "$ANDROID_KEYSTORE" \
      --ks-key-alias "$ANDROID_KEY_ALIAS" \
      --ks-pass "$ANDROID_KEYSTORE_PASS" \
      --key-pass "$ANDROID_KEY_PASS" \
      --out "$APK_OUTPUT" "$aligned"
    rm -f "$aligned" "${aligned}.idsig" 2>/dev/null || true

    # verify
    "$apksigner" verify --verbose "$APK_OUTPUT" | head -n 5 || true
    echo ""
    echo "✅  APK signed: $APK_OUTPUT"
  else
    echo ""
    echo "✅  APK built (unsigned): $APK_OUTPUT"
    echo "    To sign, set ANDROID_KEYSTORE to a JKS file."
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Disk space preflight (soft warning, exits gracefully on error)
# ────────────────────────────────────────────────────────────────────────────

check_disk_space() {
  local free_kb=""
  # Try to get free space; if df/cygpath fail, skip the check entirely.
  free_kb="$(df /c 2>/dev/null | awk 'NR==2{print $4}' || true)"
  if [[ -z "$free_kb" || ! "$free_kb" =~ ^[0-9]+$ ]]; then
    echo "⚠️  Could not check free disk space (skipping)."
    return 0
  fi
  if [[ "$free_kb" -lt 10485760 ]]; then
    echo "⚠️  WARNING: Less than 10 GB free on C: drive (~5 GB needed for NDK + SDK + JDK)."
    echo "   Free space: $((free_kb / 1024 / 1024)) GB"
    if ! confirm "Continue anyway?"; then
      echo "Aborted. Free up disk space and re-run."
      exit 0
    fi
  fi
}

# ────────────────────────────────────────────────────────────────────────────
# Main
# ────────────────────────────────────────────────────────────────────────────

echo ""
check_disk_space

echo "Install paths:"
echo "  JDK 17:         $JDK17_INSTALL_DIR"
echo "  Android NDK:    $NDK_INSTALL_DIR"
echo "  SDL2 android:   $ANDROID_DEPS_PATH"
echo "  Android SDK:    $SDK_INSTALL_DIR"
echo "  ikemen-droid:   $IKEMEN_DROID_SRC"
echo "  APK output:     $APK_OUTPUT"
if [[ -n "$ANDROID_KEYSTORE" ]] && [[ -f "$ANDROID_KEYSTORE" ]]; then
  echo "  Keystore:       $ANDROID_KEYSTORE (alias: $ANDROID_KEY_ALIAS)"
else
  echo "  Keystore:       (none — APK will be unsigned)"
fi
echo ""

if ! confirm "Proceed with full Android 11 toolchain setup + APK build?"; then
  echo "Aborted."
  exit 0
fi

install_msys2_packages
install_jdk17
install_jdk17
install_ndk
install_sdl2_android
install_libxmp_android
install_ffmpeg_android
install_sdk
setup_env
verify_installation
build_libmain
download_ikemen_droid_source
download_screenpack
build_apk
