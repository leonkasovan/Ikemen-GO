//go:build windows

// Editor window for Windows: the page is shown in an embedded Microsoft Edge
// WebView2 window (github.com/jchv/go-webview2, pure Go, no cgo) instead of
// handing the URL to an external browser. The engine keeps running; the window
// lives on its own locked OS thread with its own message loop.
//
// Requirements and limits:
//   - The WebView2 Evergreen runtime must be installed (Windows 11 and any
//     Windows 10 with Edge have it). editorWebViewRuntimeInstalled() checks the
//     registry first, because the binding calls log.Fatal if the runtime is
//     present but unusable, which would take the game down with it.
//   - This binding creates a top level window; it cannot be re-parented into
//     the SDL window (the cgo bindings of the WebView2 SDK could, but they need
//     the SDK headers, which the engine does not vendor).
//   - editorWebViewOpen() returning false means "no built in window", so the
//     caller falls back to the default browser.

package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

// editorWebView2Client is the registry key of the WebView2 Evergreen runtime.
const editorWebView2Client = "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

// editorWebViewInitTimeout bounds the wait for the WebView2 environment, which
// is created on the webview thread and can block for a while on a cold start.
const editorWebViewInitTimeout = 20 * time.Second

var (
	editorWebViewMu      sync.Mutex
	editorWebView        webview2.WebView
	editorWebViewView    string
	editorWebViewOpening bool
)

// editorWebViewRuntimeInstalled reports whether the WebView2 runtime is
// registered for this machine.
func editorWebViewRuntimeInstalled() bool {
	key := `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + editorWebView2Client
	// registry.WOW64_64KEY / WOW64_32KEY are KEY_* access bits, so they have to be
	// combined with the value access we need.
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY, 0} {
			k, err := registry.OpenKey(root, key, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			found := editorWebViewVersion(k)
			k.Close()
			if found {
				return true
			}
		}
	}
	return false
}

// editorWebViewVersion reads the runtime version of an EdgeUpdate client key.
// Edge writes "pv" as a string on some installs and as a DWORD on others.
func editorWebViewVersion(k registry.Key) bool {
	if pv, _, err := k.GetIntegerValue("pv"); err == nil {
		return pv > 0
	}
	if s, _, err := k.GetStringValue("pv"); err == nil {
		s = strings.TrimSpace(s)
		return s != "" && s != "0.0.0.0"
	}
	return false
}

// editorWebViewState reports the built in window state for the status endpoint.
func editorWebViewState() string {
	editorWebViewMu.Lock()
	defer editorWebViewMu.Unlock()
	switch {
	case editorWebView != nil:
		return "open:" + editorWebViewView
	case editorWebViewOpening:
		return "opening"
	case editorWebViewRuntimeInstalled():
		return "ready"
	default:
		return "no-runtime"
	}
}

// editorWebViewDataPath is where the WebView2 profile is kept, inside the game
// folder so the editor leaves nothing behind elsewhere.
func editorWebViewDataPath() string {
	dir, err := filepath.Abs(filepath.Join(sys.baseDir, "save", "editor-webview"))
	if err != nil {
		return ""
	}
	return dir
}

// editorWebViewSize follows the game window, clamped to a usable editor size. It
// reads engine state, so it has to be called from the engine thread rather than
// from the goroutine that creates the window.
func editorWebViewSize() (uint, uint) {
	w, h := int(sys.gameWidth), int(sys.gameHeight)
	if w < 1280 {
		w = 1280
	}
	if h < 720 {
		h = 720
	}
	w = w * 4 / 5
	h = h * 4 / 5
	if w > 1920 {
		w = 1920
	}
	if h > 1200 {
		h = 1200
	}
	return uint(w), uint(h)
}

// editorWebViewOpen shows url in the built in window. A window that is already
// open is reused and navigated to the new view. Returns false when there is no
// built in window available, so the caller can use the browser instead.
func editorWebViewOpen(url, title string, width, height uint) bool {
	editorWebViewMu.Lock()
	if w := editorWebView; w != nil {
		// The window is reused, so the view it reports (and the title it shows)
		// follows the view being opened rather than staying on the first one.
		editorWebViewView = title
		editorWebViewMu.Unlock()
		// SetTitle must run on the UI thread, like Navigate.
		w.Dispatch(func() {
			w.Navigate(url)
			w.SetTitle(title)
		})
		LogMessage("[Editor] WebView2 window switched to %v", url)
		return true
	}
	if editorWebViewOpening {
		// A cold start is still in flight. Starting a second one would show the
		// editor in two windows, so this call joins the one already on its way.
		editorWebViewMu.Unlock()
		LogMessage("[Editor] the editor window is still opening")
		return true
	}
	// Owned by the goroutine below, which clears it once the window is up (or
	// has failed), so no second call can start a competing window.
	editorWebViewOpening = true
	editorWebViewMu.Unlock()

	if !editorWebViewRuntimeInstalled() {
		LogMessage("[Editor] WebView2 runtime not installed, using the browser")
		return false
	}
	ready := make(chan bool, 1)
	go func() {
		// The WebView2 environment and its message loop are bound to one thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			Debug:     false,
			DataPath:  editorWebViewDataPath(),
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title: title, Width: width, Height: height, Center: true,
			},
		})
		if w == nil {
			editorWebViewMu.Lock()
			editorWebViewOpening = false
			editorWebViewMu.Unlock()
			ready <- false
			return
		}
		editorWebViewMu.Lock()
		editorWebViewOpening = false
		editorWebView = w
		editorWebViewView = title
		editorWebViewMu.Unlock()
		w.Navigate(url)
		ready <- true
		w.Run() // message loop until the window is closed
		w.Destroy()
		editorWebViewMu.Lock()
		editorWebView = nil
		editorWebViewView = ""
		editorWebViewMu.Unlock()
		LogMessage("[Editor] WebView2 window closed")
	}()
	select {
	case ok := <-ready:
		if ok {
			LogMessage("[Editor] WebView2 window opened: %v", url)
		}
		return ok
	case <-time.After(editorWebViewInitTimeout):
		LogMessage("[Editor] the WebView2 window did not come up in time")
		return false
	}
}

// editorWebViewClose closes the built in window if one is open.
func editorWebViewClose() bool {
	editorWebViewMu.Lock()
	w := editorWebView
	editorWebViewMu.Unlock()
	if w == nil {
		return false
	}
	w.Destroy()
	return true
}
