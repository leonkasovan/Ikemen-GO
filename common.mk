# ============================================================================
# common.mk — Shared library URLs and download/extract macro
# ============================================================================
# Included by the main Makefile. Provides:
#   - Library source URLs (SDL2, FFmpeg, libxmp, screenpack)
#   - A reusable download_and_extract macro (wget + unzip, no git clone)
#
# GitHub archive URL patterns:
#   Tag:    https://github.com/<owner>/<repo>/archive/refs/tags/<tag>.zip
#   Branch: https://github.com/<owner>/<repo>/archive/refs/heads/<branch>.zip
#
# GitHub zips wrap contents in a top-level dir (e.g. SDL-release-2.32.10/).
# The macro flattens this into the destination directory.
# ============================================================================

# --- Library versions and archive URLs ---------------------------------------

SDL2_URL    := https://github.com/libsdl-org/SDL/archive/refs/tags/release-2.32.10.zip
FFMPEG_URL  := https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n7.1.zip
XMP_URL     := https://github.com/libxmp/libxmp/archive/refs/tags/libxmp-4.7.1.zip

SCREENPACK_URL := https://github.com/ikemen-engine/Ikemen-GO-Screenpack/archive/refs/heads/master.zip

# --- download_and_extract macro ---------------------------------------------
# Usage: $(call download_and_extract,URL,ZIP_PATH,DEST_DIR)
#
# Downloads a zip archive from URL to ZIP_PATH, extracts it, flattens the
# top-level directory into DEST_DIR, and cleans up. Skips if DEST_DIR exists.
#
# Escaping notes (Make define + $(call) recipe expansion):
#   $(1)/$(2)/$(3)  — Make call params (expanded by $(call ...))
#   $$var           — shell variable  ($$ → $ after expansion)
#   $$(cmd)         — command substitution ($$ → $, so $(cmd) in shell)
#   $${var%...}     — parameter expansion ($$ → $, then ${var%...} in shell)
#
# IMPORTANT: In a `define`d variable expanded via `$(call ...)` in a recipe,
# each `$$` produces one `$`. So `$$tmp` becomes `$tmp` (a shell variable).
# Do NOT use `$$$$tmp` — that would produce `$$tmp` (PID-based temp name).
define download_and_extract
	if [ ! -d "$(3)" ]; then
		echo "==> Downloading $(1)..."
# check $(2) for existence to avoid re-downloading if the zip already exists
		if [ ! -f "$(2)" ]; then
			wget -q "$(1)" -O "$(2)"
		else
			echo "==> Using existing zip: $(2)"
		fi
		tmp="$(2)-extract"
		rm -rf "$$tmp"
		mkdir -p "$$tmp"
		unzip -q "$(2)" -d "$$tmp"
		subdir="$$(find "$$tmp" -mindepth 1 -maxdepth 1 -type d | head -1)"
		rm -rf "$(3)"
		mkdir -p "$(3)"
		shopt -s dotglob
		mv "$$subdir"/* "$(3)"/ 2>/dev/null || true
		rm -rf "$$tmp" "$(2)"
	fi
endef