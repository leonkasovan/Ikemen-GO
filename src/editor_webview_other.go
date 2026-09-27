//go:build !windows

// Editor window for every platform without a WebView2 binding: the caller
// falls back to the default web browser. The WebView2 implementation lives in
// editor_webview_windows.go.

package main

// editorWebViewOpen reports that there is no built in window here.
func editorWebViewOpen(url, title string, width, height uint) bool { return false }

// editorWebViewSize follows the game window. There is no built in window on this
// platform, so the size is never used; it exists so the caller can read it on the
// engine thread the same way everywhere.
func editorWebViewSize() (uint, uint) { return 0, 0 }

// editorWebViewState reports the built in window state for the status endpoint.
func editorWebViewState() string { return "unsupported" }

// editorWebViewClose closes the built in window if one is open.
func editorWebViewClose() bool { return false }
