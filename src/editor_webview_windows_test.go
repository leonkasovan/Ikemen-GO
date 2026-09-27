//go:build windows

// The built in editor window needs a desktop session, so the test that actually
// opens it is opt in: IKEMEN_TEST_WEBVIEW=1 go test ./src -run EditorWebView.

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestEditorWebViewRuntimeProbe(t *testing.T) {
	// The probe must never panic, whatever the machine looks like.
	t.Logf("WebView2 runtime installed: %v (%v)", editorWebViewRuntimeInstalled(), editorWebViewState())
	if editorWebViewRuntimeInstalled() && editorWebViewState() != "ready" {
		t.Errorf("state = %q with the runtime installed, want %q", editorWebViewState(), "ready")
	}
}

// The Lua openEditor() has to return straight away: the built-in window is
// created on its own thread and used to block the engine thread for up to
// editorWebViewInitTimeout on a cold start, which froze the game.
func TestEditorOpenAsyncDoesNotBlock(t *testing.T) {
	oldBase := sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.baseDir = oldBase }()
	if editorServiceURL("motif") == "" {
		// The service binds the real port, so a running game (or a leftover one)
		// makes this test meaningless rather than wrong.
		t.Skipf("the editor service could not start on port %d (is the game running?)", EditorPort)
	}
	start := time.Now()
	ok := openEditorAsync("motif")
	elapsed := time.Since(start)
	if !ok {
		t.Error("openEditorAsync = false, want the request to be accepted")
	}
	// Well under the cold start timeout: the window is created in the background.
	if elapsed > time.Second {
		t.Errorf("openEditorAsync took %v, it must not wait for the window", elapsed)
	}
	// Whatever the platform does next, the caller is free: a second request is
	// accepted too, and must not open a competing window.
	if !openEditorAsync("stage") {
		t.Error("a second openEditorAsync = false")
	}
	// Let the background window come up and close it again, so this test leaves
	// no window behind for the next one to trip over.
	deadline := time.Now().Add(editorWebViewInitTimeout)
	for time.Now().Before(deadline) {
		editorWebViewMu.Lock()
		busy := editorWebView != nil || editorWebViewOpening
		editorWebViewMu.Unlock()
		if !busy {
			return
		}
		if editorWebViewState() != "opening" {
			editorWebViewClose()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the background window never settled")
}

// A cold start in flight must not be started twice, or the editor would open in
// two windows.
func TestEditorWebViewOpenIsSingleFlight(t *testing.T) {
	if !editorWebViewRuntimeInstalled() {
		t.Skip("no WebView2 runtime on this machine")
	}
	oldBase := sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.baseDir = oldBase }()
	w, h := editorWebViewSize()

	// Stand in for a cold start that has not finished coming up yet.
	editorWebViewMu.Lock()
	busy := editorWebView != nil || editorWebViewOpening
	editorWebViewOpening = true
	editorWebViewMu.Unlock()
	if busy {
		t.Skip("an editor window is already open or opening")
	}
	opened := editorWebViewOpen(editorServiceURL("stage"), editorViewTitle("stage"), w, h)

	editorWebViewMu.Lock()
	created := editorWebView != nil
	editorWebViewOpening = false
	editorWebViewMu.Unlock()
	if !opened {
		t.Error("a second request while the window is opening = false, want it accepted")
	}
	if created {
		editorWebViewClose()
		t.Error("a second window was started while one was already opening")
	}
}

func TestEditorWebViewOpen(t *testing.T) {
	if os.Getenv("IKEMEN_TEST_WEBVIEW") == "" {
		t.Skip("set IKEMEN_TEST_WEBVIEW=1 to open the real WebView2 window")
	}
	if !editorWebViewRuntimeInstalled() {
		t.Skip("no WebView2 runtime on this machine")
	}
	oldBase := sys.baseDir
	// Not t.TempDir(): the WebView2 browser process can still hold its profile
	// files for a moment after the window is closed, which would make the
	// automatic cleanup fail.
	dir, err := os.MkdirTemp("", "ikemen-webview-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sys.baseDir = dir
	defer func() { sys.baseDir = oldBase }()

	url := editorServiceURL("motif")
	if url == "" {
		t.Fatal("the editor service did not start")
	}
	w0, h0 := editorWebViewSize()
	if !editorWebViewOpen(url, editorViewTitle("motif"), w0, h0) {
		t.Fatal("editorWebViewOpen() = false, want the built in window")
	}
	if got := editorWebViewState(); !strings.HasPrefix(got, "open:") {
		t.Fatalf("state = %q, want an open window", got)
	}

	// The window really exists and is on screen.
	editorWebViewMu.Lock()
	w := editorWebView
	editorWebViewMu.Unlock()
	if w == nil {
		t.Fatal("no webview was stored")
	}
	hwnd := windows.HWND(uintptr(w.Window()))
	t.Logf("WebView2 window %v, title %q, url %v", uintptr(hwnd), editorViewTitle("motif"), url)
	if !windows.IsWindow(hwnd) {
		t.Error("the webview window handle is not a window")
	}
	if !windows.IsWindowVisible(hwnd) {
		t.Error("the webview window is not visible")
	}

	// Asking for another view reuses the window instead of opening a new one.
	w1, h1 := editorWebViewSize()
	if !editorWebViewOpen(editorServiceURL("stage"), editorViewTitle("stage"), w1, h1) {
		t.Error("reopening the editor window failed")
	}
	editorWebViewMu.Lock()
	again := editorWebView
	editorWebViewMu.Unlock()
	if again != w {
		t.Error("a second webview was created instead of reusing the window")
	}

	if !editorWebViewClose() {
		t.Error("editorWebViewClose() = false, want the window to close")
	}
	for i := 0; i < 100; i++ {
		if !strings.HasPrefix(editorWebViewState(), "open:") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if strings.HasPrefix(editorWebViewState(), "open:") {
		t.Error("the window is still open after editorWebViewClose()")
	}
	if editorWebViewClose() {
		t.Error("editorWebViewClose() = true with no window open")
	}
}
