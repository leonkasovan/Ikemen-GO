package main

// Built-in editor HTTP service (command line flag: -httpservice, port 6700).
//
// The service exposes a local-only (127.0.0.1) HTTP API plus a small web UI that
// breaks the currently configured motif / stage / character definitions down
// structurally, the same way the engine itself parses them:
//
//   - src/motif.go maps the motif INI onto the Motif struct (INI section ->
//     dotted key -> struct field, including `default` tags). The reflection
//     walk in editorMotifStructure() reports exactly that layout.
//   - the background definitions are not in the struct: bgdef.go collects the
//     "[<bgname> <layer>]" and "<bgname>ctrl" sections of the .def and reads
//     them with readBackGround / bgCtrl.read (src/stage.go) at load time.
//     editorBGElementSchema() and editorBGCtrlSchema() list those keys, so the
//     layers and controllers are expanded with a type and a default as well.
//   - the sections are grouped by screen the way the Motif struct declares
//     them, with the layers of every <name>BGdef below their background
//     definition and each [Begin Action n] under the layer that plays it.
//   - select.def drives the stage / character lists, mirroring the parsing in
//     external/script/main.lua (main.f_parseSelectDef section handling). Both
//     .def views pick a file from a combo box and show its sections as a tree,
//     the same way the Motif view does.
//
// The service is started at boot when -httpservice is passed, and on demand by
// the in-game Editor menu (Lua global openEditor(view)), which also opens the
// page in the default web browser.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/ini.v1"
)

const (
	// EditorPort is the TCP port used by the built-in editor HTTP service.
	EditorPort = 6700
	// editorMaxBody caps the size of request bodies accepted by the editor API.
	editorMaxBody = 1 << 20
	// editorMaxReadFile caps the size of a file the editor reads whole (through
	// /api/file or the motif / INI helpers). A stat check refuses anything
	// larger before it is read, so a huge in-sandbox blob cannot OOM the
	// process. 16 MiB fits any real .def / .ini / motif with room to spare.
	editorMaxReadFile = 16 << 20
	// editorMaxSFF caps the size of a .sff the editor decodes for the sprite
	// preview. A .sff is read and decoded whole, so a huge one is refused the
	// same way.
	editorMaxSFF = 256 << 20
)

var (
	editorMu      sync.Mutex
	editorRunning bool
	editorBaseURL string

	// editorSaveMu serializes /api/save requests. A save reads a file, edits it
	// line by line and writes it back, so two concurrent requests could lose an
	// edit, and their live applies could run in a different order than they were
	// written to disk.
	editorSaveMu sync.Mutex

	editorDefaultsOnce sync.Once
	editorDefaultsINI  *ini.File

	// editorStructureOnce memoises editorMotifStructure. The walk reflects over
	// reflect.TypeOf(Motif{}), so its result depends only on the compiled type
	// and is identical for every call, yet it is the entire response schema:
	// recomputing it twice per /api/motif was the single most expensive step of
	// the read path (~7 ms and ~2 MB of garbage each time). The cached slice is
	// shared, so callers must treat it as read-only.
	editorStructureOnce sync.Once
	editorStructure     []editorSchemaSectionJSON
)

// startEditorHTTPService starts the editor HTTP service exactly once and returns
// its base URL (or "" when the port is unavailable). It never blocks.
func startEditorHTTPService() string {
	editorMu.Lock()
	defer editorMu.Unlock()
	if editorRunning {
		return editorBaseURL
	}
	addr := fmt.Sprintf("127.0.0.1:%d", EditorPort)
	url := fmt.Sprintf("http://%s/", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", editorHandlePage)
	mux.HandleFunc("/api/status", editorHandleStatus)
	mux.HandleFunc("/api/motif", editorHandleMotif)
	mux.HandleFunc("/api/stages", editorHandleStages)
	mux.HandleFunc("/api/characters", editorHandleCharacters)
	mux.HandleFunc("/api/file", editorHandleFile)
	mux.HandleFunc("/api/sff", editorHandleSFF)
	mux.HandleFunc("/api/save", editorHandleSave)
	mux.HandleFunc("/api/reload", editorHandleReload)
	mux.HandleFunc("/api/pins", editorHandlePins)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		LogMessage("[Editor] unable to listen on %v: %v", addr, err)
		return ""
	}
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	editorRunning = true
	editorBaseURL = url
	go func() {
		LogMessage("[Editor] HTTP service listening on %v", url)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			LogMessage("[Editor] HTTP service stopped: %v", err)
		}
	}()
	return url
}

// editorServiceBaseURL returns the base URL of the running editor service, or
// "" when it is not up. It takes editorMu, because the listener goroutine may
// have set the URL while a status request is being served.
func editorServiceBaseURL() string {
	editorMu.Lock()
	defer editorMu.Unlock()
	return editorBaseURL
}

// editorNormalizeView clamps a view name to the supported editor tabs.
func editorNormalizeView(view string) string {
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "stage":
		return "stage"
	case "character":
		return "character"
	default:
		return "motif"
	}
}

// editorServiceURL ensures the service is running and returns the URL of the
// requested editor view ('motif', 'stage' or 'character').
func editorServiceURL(view string) string {
	base := startEditorHTTPService()
	if base == "" {
		return ""
	}
	return base + "?view=" + editorNormalizeView(view)
}

// openEditorAsync shows a view without blocking the caller, which is what the Lua
// openEditor() goes through. Starting the service is quick, but the built in
// WebView2 window is created on its own thread and can take a while to come up on
// a cold start, so waiting for it on the engine thread froze the game for up to
// editorWebViewInitTimeout while the menu item was picked.
//
// Returns false only when there is nothing to show at all, so the caller can
// fall back to telling the user the address.
func openEditorAsync(view string) bool {
	url := editorServiceURL(view)
	if url == "" {
		return false
	}
	title := editorViewTitle(view)
	// The size follows the game window, which the engine thread owns and can
	// change on a resolution switch: read it here, on that thread, rather than
	// from the goroutine below.
	width, height := editorWebViewSize()
	go func() {
		if !openEditorView(url, title, width, height) {
			LogMessage("[Editor] unable to show %v", url)
		}
	}()
	return true
}

// editorViewTitle is the window title of the editor for a view.
func editorViewTitle(view string) string {
	view = editorNormalizeView(view)
	return "Ikemen GO Editor - " + strings.ToUpper(view[:1]) + view[1:]
}

// openEditorView shows the editor page in the built in WebView2 window when the
// platform has one (Windows), and falls back to the default web browser. It runs
// on its own goroutine, so it may block on a cold window without stalling the
// game; the size is passed in for the same reason.
func openEditorView(url, title string, width, height uint) bool {
	if editorWebViewOpen(url, title, width, height) {
		return true
	}
	if err := openExternalURL(url); err != nil {
		LogMessage("[Editor] unable to open a browser for %v: %v", url, err)
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Path / INI helpers
// ---------------------------------------------------------------------------

// editorINIOptions returns the load options shared by all editor INI reads.
func editorINIOptions() ini.LoadOptions {
	return ini.LoadOptions{
		Insensitive:             false,
		InsensitiveSections:     true,
		InsensitiveKeys:         false,
		IgnoreInlineComment:     false,
		SkipUnrecognizableLines: true,
		UnparseableSections:     []string{"Infobox Text"},
		PreserveSurroundedQuote: true,
	}
}

// editorRootDir returns the absolute path of the game folder.
func editorRootDir() string {
	dir, err := filepath.Abs(sys.baseDir)
	if err != nil || dir == "" {
		return "."
	}
	return dir
}

// editorSandboxPath resolves p (relative to the game folder, or an absolute
// path inside it) and refuses anything pointing outside the game folder.
func editorSandboxPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	root := editorRootDir()
	abs := filepath.FromSlash(p)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside the game folder: %v", p)
	}
	return abs, nil
}

// editorConfigPath returns the engine config file path (-config aware).
func editorConfigPath() string {
	if v, ok := sys.cmdFlags["-config"]; ok && strings.TrimSpace(v) != "" {
		return v
	}
	return "save/config.ini"
}

// editorINIKeyValue returns a key value, matching the name case-insensitively.
func editorINIKeyValue(sec *ini.Section, name string) string {
	if sec == nil {
		return ""
	}
	for _, k := range sec.Keys() {
		if strings.EqualFold(k.Name(), name) {
			return strings.TrimSpace(k.Value())
		}
	}
	return ""
}

// editorMotifPath returns the motif .def used by the engine ([Config] Motif in
// the engine config, overridable with -r), resolved to an existing file.
func editorMotifPath() string {
	name := ""
	if v, ok := sys.cmdFlags["-r"]; ok {
		name = strings.TrimSpace(v)
	}
	if name == "" {
		cfgPath := editorConfigPath()
		if _, err := editorFileWithinCap(cfgPath, editorMaxReadFile); err == nil {
			if cfg, _, err := LoadINIFile(cfgPath, editorINIOptions()); err == nil && cfg != nil {
				if sec, err := cfg.GetSection("Config"); err == nil {
					name = editorINIKeyValue(sec, "Motif")
				}
			}
		}
	}
	if name == "" {
		return ""
	}
	return SearchFile(name, []string{name, "", "data/"})
}

// editorDefaultINI returns the embedded default motif INI (parsed once).
func editorDefaultINI() *ini.File {
	editorDefaultsOnce.Do(func() {
		f, err := LoadINIText(preprocessINIContent(NormalizeNewlines(string(defaultMotif))), editorINIOptions())
		if err != nil {
			LogMessage("[Editor] unable to parse default motif: %v", err)
			return
		}
		editorDefaultsINI = f
	})
	return editorDefaultsINI
}

// editorSelectDefPath resolves the select.def used by the configured motif.
func editorSelectDefPath() string {
	name := ""
	if motif := editorMotifPath(); motif != "" {
		if _, err := editorFileWithinCap(motif, editorMaxReadFile); err == nil {
			if f, _, err := LoadINIFile(motif, editorINIOptions()); err == nil && f != nil {
				if sec, err := f.GetSection("Files"); err == nil {
					name = editorINIKeyValue(sec, "select")
				}
			}
		}
	}
	if name == "" {
		name = "select.def"
	}
	return SearchFile(name, []string{editorMotifPath(), ""}, "data/")
}

// editorSectionTitle converts an INI tag ("title_info") to its section header
// ("Title Info"), matching the engine's section name normalization.
func editorSectionTitle(tag string) string {
	parts := strings.Split(tag, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// editorKeyChoices maps unambiguous enumerated INI keys to the values the
// engine accepts, so the editor can offer a drop-down instead of a free text
// field. The lists come from the parsers: compiler.go (trans, projection,
// space, savedata), font.go (banktype) and iniutils.go (textwrap).
var editorKeyChoices = map[string][]string{
	// [Begin Action] / state controllers.
	"trans":      {"default", "none", "add", "add1", "addalpha", "sub", "subadd"},
	"projection": {"orthographic", "perspective", "perspective2"},
	"space":      {"stage", "screen"},
	"savedata":   {"map", "var", "fvar"},
	// Text wrapping: "w" (or "1") wraps, anything else does not.
	"textwrap": {"w", "1", "none"},
	// Font definitions.
	"banktype": {"palette", "sprite"},
	// Video backgrounds: stage.go readBackGround.
	"scalemode":   {"none", "stretch", "fit", "fitwidth", "fitheight", "zoomfill"},
	"scalefilter": {"fastbilinear", "bilinear", "bicubic", "experimental", "neighbor", "area", "bicublin", "gauss", "sinc", "lanczos", "spline"},
}

// "type" means something different depending on the section it lives in, so it
// is resolved from the section and file instead of the plain key name.
var (
	// font.go loadDefInfo (font .def files, [Files] font<n>.type of the motif)
	editorFontTypeChoices = []string{"truetype", "bitmap"}
	// stage.go readBackGround ([BG n] and the layers of a <name>BGdef block)
	editorBGTypeChoices = []string{"normal", "anim", "parallax", "video", "dummy"}
	// stage.go bgCtrl.read ([StageInfo], [PlayerInfo], [Bound], â€¦)
	editorBGCtrlTypeChoices = []string{
		"anim", "visible", "enable", "null", "palfx",
		"posset", "posadd", "remappal", "sinx", "siny", "velset", "veladd",
	}
	// Sections holding a background controller "type".
	editorBGCtrlSections = map[string]bool{
		"stageinfo": true, "playerinfo": true, "bound": true,
		"shadow": true, "reflection": true, "camera": true,
	}
	// Sections only found in a font .def file.
	editorFontSections = map[string]bool{
		"fnt v1": true, "fnt v2": true, "sizes": true, "char": true,
	}
)

// editorDynamicKeyMatch reports whether a key of a .def file belongs to a
// dynamic (map valued) schema entry. Map entries are written in several shapes,
// so all of them are recognized:
//
//	menu.itemname.editor        flattened dotted form  (plain map)
//	font1                       bare map key           ([Files] font<n>)
//	textinput.aspectheight.text name first form         (keyfirst text map)
func editorDynamicKeyMatch(schemaKey, fileKey string) bool {
	base := strings.TrimSuffix(schemaKey, ".<name>")
	if strings.HasPrefix(fileKey, base+".") || fileKey == base {
		return true
	}
	name := editorFirstToken(base)
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return false
	}
	// Bare map key of a map at the section root, e.g. "[Files] font1". Only
	// undotted keys qualify, so a nested key cannot be swallowed by name.
	if !strings.Contains(fileKey, ".") && strings.HasPrefix(fileKey, name) {
		return true
	}
	// keyfirst form "<prefix>.<name>.<field>", e.g. "textinput.aspectheight.text".
	return strings.HasSuffix(fileKey, "."+name)
}

// editorChoicesFor returns the values the engine accepts for an INI key, or nil
// when the key is a free form value. Keys are matched on their last segment, so
// "trans" and "p1.face.trans" resolve to the same list; "type" additionally
// depends on the section and file it belongs to (font vs. background).
func editorChoicesFor(path, section, key string) []string {
	base := strings.ToLower(strings.TrimSpace(key))
	if i := strings.LastIndex(base, "."); i >= 0 {
		base = base[i+1:]
	}
	if base != "type" {
		return editorKeyChoices[base]
	}

	sec := strings.ToLower(strings.TrimSpace(section))
	rest := sec
	if i := strings.Index(sec, " "); i >= 0 {
		rest = strings.TrimSpace(sec[i+1:])
	}
	// Layers and controllers of a <name>BGdef block (motif backgrounds) and of a
	// stage .def ("[BG 0]", "[BG0Ctrl Credits]").
	if editorBGDefHead(sec) != "" {
		head := editorSectionHead(sec)
		if strings.HasSuffix(head, "ctrl") || strings.HasSuffix(head, "ctrldef") ||
			editorBGCtrlSections[rest] || editorBGCtrlSections[sec] {
			return editorBGCtrlTypeChoices
		}
		return editorBGTypeChoices
	}
	// The controller sections a stage .def lists by name.
	if editorBGCtrlSections[sec] {
		return editorBGCtrlTypeChoices
	}
	switch {
	// Font definitions: a font .def file, a font specific section, or the
	// [Files] font<n>.type entries of a motif.
	case editorIsFontFile(path) || editorFontSections[sec] || editorFontSections[rest]:
		return editorFontTypeChoices
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(key)), "font"):
		return editorFontTypeChoices
	}
	return nil
}

// editorFirstToken returns the first whitespace separated word of s.
func editorFirstToken(s string) string {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// editorIsFontFile reports whether a path points at a font .def file.
// The path may be relative to the game folder or absolute, so it is normalized
// to a leading slash before looking for the font directory.
func editorIsFontFile(path string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	p = "/" + strings.TrimPrefix(p, "/")
	return strings.Contains(p, "/font/") || strings.Contains(p, "/fonts/")
}

// ---------------------------------------------------------------------------
// HTTP plumbing
// ---------------------------------------------------------------------------

// editorWriteJSON marshals v as indented JSON into the response.
func editorWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		LogMessage("[Editor] json encode error: %v", err)
	}
}

// editorWriteError reports a failed request in JSON form.
func editorWriteError(w http.ResponseWriter, code int, format string, a ...any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": fmt.Sprintf(format, a...),
	})
}

// editorReadJSONRequest is the shared preamble of the mutating endpoints: it
// enforces POST (when requirePost), the X-Editor-Request header that forces a
// CORS preflight, and a body within editorMaxBody, then returns the raw JSON
// body. It reports false after writing the error response, so a handler just
// returns when the second result is false.
func editorReadJSONRequest(w http.ResponseWriter, r *http.Request, requirePost bool) ([]byte, bool) {
	if requirePost && r.Method != http.MethodPost {
		editorWriteError(w, http.StatusMethodNotAllowed, "POST required")
		return nil, false
	}
	// The custom header forces a CORS preflight, blocking cross-site writes.
	if r.Header.Get("X-Editor-Request") == "" {
		editorWriteError(w, http.StatusForbidden, "missing X-Editor-Request header")
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, editorMaxBody))
	if err != nil {
		editorWriteError(w, http.StatusBadRequest, "unable to read body: %v", err)
		return nil, false
	}
	return body, true
}

// openExternalURL launches the platform default handler for the given URL.
func openExternalURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ---------------------------------------------------------------------------
// Structural breakdown of the motif (src/motif.go -> INI layout)
// ---------------------------------------------------------------------------

// editorSchemaKeyJSON is one INI key supported by a motif section.
type editorSchemaKeyJSON struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Dynamic bool   `json:"dynamic,omitempty"`
}

// editorSchemaSectionJSON describes one motif INI section and the keys it accepts.
type editorSchemaSectionJSON struct {
	Section string `json:"section"`
	Title   string `json:"title"`
	Field   string `json:"field"`
	// Kind is "struct" for a field backed by the Motif struct, "map" when the
	// section name itself is user defined (a map field) and "dynamic" for a
	// fixed section holding user named keys ([Music]).
	Kind    string                `json:"kind,omitempty"`
	Pattern string                `json:"pattern,omitempty"`
	Keys    []editorSchemaKeyJSON `json:"keys"`
}

// editorMusicSchema describes [Music], which the engine parses with its own
// key splitter (iniutils.go splitMusicKey) instead of a struct tag: every key is
// "<entry>.bgm" plus the optional properties of the same entry.
func editorMusicSchema() editorSchemaSectionJSON {
	// Defaults come from BgmProperties in motif.go.
	props := []struct {
		name string
		typ  string
		def  string
	}{
		{"bgm", "string", ""},
		{"loop", "[]int32", "1"},
		{"volume", "[]int32", "100"},
		{"loopstart", "[]int32", ""},
		{"loopend", "[]int32", ""},
		{"startposition", "[]int32", ""},
		{"freqmul", "[]float32", "1"},
		{"loopcount", "[]int32", "-1"},
	}
	sch := editorSchemaSectionJSON{
		Section: "music", Title: "Music", Field: "Music", Kind: "dynamic",
		Keys: []editorSchemaKeyJSON{},
	}
	for _, p := range props {
		sch.Keys = append(sch.Keys, editorSchemaKeyJSON{
			Key: "music.<name>." + p.name, Type: p.typ, Default: p.def, Dynamic: true,
		})
	}
	return sch
}

// editorSchemaForMapField describes a map field of the Motif struct: the section
// name matches Pattern and the keys are those of the map's element struct.
func editorSchemaForMapField(f reflect.StructField, tag string) (editorSchemaSectionJSON, bool) {
	pattern := strings.TrimSpace(strings.TrimPrefix(tag, "map:"))
	elem := f.Type
	if elem.Kind() == reflect.Map {
		elem = elem.Elem()
	}
	if elem.Kind() == reflect.Ptr {
		elem = elem.Elem()
	}
	if pattern == "" || elem.Kind() != reflect.Struct {
		return editorSchemaSectionJSON{}, false
	}
	keys := []editorSchemaKeyJSON{}
	editorCollectSchemaKeys("", elem, &keys, 0)
	if len(keys) == 0 {
		return editorSchemaSectionJSON{}, false
	}
	return editorSchemaSectionJSON{
		Section: f.Name, Title: f.Name, Field: f.Name,
		Kind: "map", Pattern: pattern, Keys: keys,
	}, true
}

// editorBGElementSchema describes one layer of a background definition:
// "[TitleBG Background Sky]", "[VersusBG 5]", "[TitleBG 5]", ...
// These sections are not part of the Motif struct. bgdef.go collects every
// "[<bgname> <layer>]" section of the .def and hands it to readBackGround
// (stage.go) when the background is loaded, so the keys are listed here from
// that parser. The defaults are the values newBackGround() starts from.
func editorBGElementSchema() editorSchemaSectionJSON {
	keys := []struct {
		name string
		typ  string
		def  string
	}{
		{"type", "string", ""},
		{"layerno", "int32", "0"},
		{"actionno", "int32", "-1"},
		{"spriteno", "[2]int32", ""},
		{"mask", "int32", "-1"},
		{"positionlink", "bool", "false"},
		{"autoresizeparallax", "bool", "false"},
		{"start", "[2]float32", "0, 0"},
		{"delta", "[2]float32", "1, 1"},
		{"scalestart", "[2]float32", "1, 1"},
		{"scaledelta", "[2]float32", "0, 0"},
		{"xshear", "float32", "0"},
		{"angle", "float32", "0"},
		{"xangle", "float32", "0"},
		{"yangle", "float32", "0"},
		{"focallength", "float32", "2048"},
		{"projection", "string", "orthographic"},
		{"zoomdelta", "[2]float32", ""},
		{"zoomscaledelta", "[2]float32", ""},
		{"xbottomzoomdelta", "float32", ""},
		{"trans", "string", "default"},
		{"alpha", "[2]int32", "255, 0"},
		{"tile", "[2]int32", "0, 0"},
		{"tilespacing", "[2]int32", "0, 0"},
		{"window", "[4]int32", "-32768, -32768, 65535, 65535"},
		{"maskwindow", "[4]int32", "-32768, -32768, 65535, 65535"},
		{"windowdelta", "[2]float32", "0, 0"},
		{"id", "int32", ""},
		{"velocity", "[2]float32", "0, 0"},
		{"sin.x", "[3]float32", ""},
		{"sin.y", "[3]float32", ""},
		{"roundpos", "bool", ""},
		{"shader", "string", ""},
		// Video layers only.
		{"path", "string", ""},
		{"volume", "int32", "100"},
		{"scalemode", "string", "none"},
		{"scalefilter", "string", "fastbilinear"},
		{"loop", "bool", "false"},
		// Parallax layers only.
		{"width", "[2]int32", ""},
		{"xscale", "[2]float32", "1, 1"},
		{"yscalestart", "float32", "100"},
		{"yscaledelta", "float32", "0"},
	}
	sch := editorSchemaSectionJSON{
		Section: "bg", Title: "BG layer", Kind: "runtime", Keys: []editorSchemaKeyJSON{},
	}
	for _, k := range keys {
		sch.Keys = append(sch.Keys, editorSchemaKeyJSON{Key: k.name, Type: k.typ, Default: k.def})
	}
	// Shader parameters are user named: shaderparam.p0 ... shaderparam.p15.
	sch.Keys = append(sch.Keys, editorSchemaKeyJSON{
		Key: "shaderparam.<name>", Type: "float32", Dynamic: true,
	})
	return sch
}

// editorBGCtrlSchema describes a background controller: "[TitleBGctrl ...]",
// "[TitleBGctrldef]", "[BG0Ctrl Credits]", "[StageInfo]", "[PlayerInfo]", ...
// bgdef.go / stage.go read them with bgCtrl.read (stage.go), which is where the
// keys and their defaults come from.
func editorBGCtrlSchema() editorSchemaSectionJSON {
	keys := []struct {
		name string
		typ  string
		def  string
	}{
		{"type", "string", ""},
		{"time", "[3]int32", "0, 0, -1"},
		{"positionlink", "bool", "false"},
		{"value", "[3]int32", ""},
		{"x", "float32", ""},
		{"y", "float32", ""},
		{"source", "[2]int32", "-1, 0"},
		{"dest", "[2]int32", "-1, 0"},
		{"add", "[3]int32", "0, 0, 0"},
		{"mul", "[3]int32", "256, 256, 256"},
		{"sinadd", "[4]int32", "0, 0, 0, 0"},
		{"sinmul", "[4]int32", "0, 0, 0, 0"},
		{"sincolor", "[2]int32", "0, 0"},
		{"sinhue", "[2]int32", "0, 0"},
		{"invertall", "int32", "0"},
		{"invertblend", "int32", "0"},
		{"color", "float32", "1"},
		{"hue", "float32", "0"},
		{"sctrlid", "int32", ""},
		// The controller definitions name the layers they drive.
		{"ctrlid", "[]int32", ""},
	}
	sch := editorSchemaSectionJSON{
		Section: "bgctrl", Title: "BG controller", Kind: "runtime", Keys: []editorSchemaKeyJSON{},
	}
	for _, k := range keys {
		sch.Keys = append(sch.Keys, editorSchemaKeyJSON{Key: k.name, Type: k.typ, Default: k.def})
	}
	return sch
}

// editorIsBGName reports whether a section head names a background block: the
// <name>BG of a motif ("titlebg", "versusbg", ...) or the "bg0" / "bg1" blocks a
// stage .def uses.
func editorIsBGName(head string) bool {
	if head == "" {
		return false
	}
	if head == "bg" || strings.HasSuffix(head, "bg") {
		return true
	}
	if !strings.HasPrefix(head, "bg") {
		return false
	}
	for _, r := range strings.TrimPrefix(head, "bg") {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// editorBGDefHead returns the background block a runtime section belongs to,
// lowercased: "[TitleBG Background Sky]" -> "titlebg", "[TitleBGctrl Fade]" ->
// "titlebg", "[BG 0]" -> "bg". It returns "" for every section the engine does
// not parse at load time.
func editorBGDefHead(name string) string {
	head := editorSectionHead(name)
	switch {
	case strings.HasSuffix(head, "ctrldef"):
		head = strings.TrimSuffix(head, "ctrldef")
	case strings.HasSuffix(head, "ctrl"):
		head = strings.TrimSuffix(head, "ctrl")
	}
	if !editorIsBGName(head) {
		return ""
	}
	return head
}

// motifBGBlock returns the <name>BGdef block a motif section belongs to, or ""
// when the section is not one of a background definition. Unlike
// editorBGDefHead it ignores the plain "bg" block of a stage .def, which has no
// BGdef section of its own.
func motifBGBlock(name string) string {
	head := editorBGDefHead(name)
	if head == "bg" {
		return ""
	}
	return head
}

// editorRuntimeSchema returns the schema of a section the engine parses at load
// time instead of through the Motif struct: the layers of a <name>BGdef block
// and the background controllers. It returns nil for every other section.
func editorRuntimeSchema(name string) *editorSchemaSectionJSON {
	head := editorBGDefHead(name)
	switch {
	case head == "":
		return nil
	case strings.HasSuffix(editorSectionHead(name), "ctrldef"),
		strings.HasSuffix(editorSectionHead(name), "ctrl"):
		// "[TitleBGctrl ...]", "[TitleBGctrldef]", ...
		sch := editorBGCtrlSchema()
		return &sch
	default:
		// "[TitleBG Background Sky]", "[VersusBG 5]", ...
		sch := editorBGElementSchema()
		return &sch
	}
}

// editorMotifStructure returns the memoised result of buildEditorMotifStructure.
// The returned slice is shared by every caller and must not be modified.
func editorMotifStructure() []editorSchemaSectionJSON {
	editorStructureOnce.Do(func() {
		editorStructure = buildEditorMotifStructure()
	})
	return editorStructure
}

// buildEditorMotifStructure walks the Motif struct and reports the INI layout
// that motif.go is able to assign: section -> dotted key -> field type/default.
// It also covers the two parsers that are not struct driven: the [Music] key
// splitter and the map fields whose section name comes from the file.
func buildEditorMotifStructure() []editorSchemaSectionJSON {
	out := []editorSchemaSectionJSON{}
	t := reflect.TypeOf(Motif{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" || f.Anonymous {
			continue
		}
		tag := strings.TrimSpace(f.Tag.Get("ini"))
		if tag == "" || tag == "def" {
			// Motif.Music has no tag: it is filled by parseMusicSection.
			if f.Name == "Music" {
				out = append(out, editorMusicSchema())
			}
			continue
		}
		if strings.HasPrefix(tag, "map:") {
			// A pattern keyed map: the section name is user defined.
			if sch, ok := editorSchemaForMapField(f, tag); ok {
				out = append(out, sch)
			}
			continue
		}
		if f.Type.Kind() != reflect.Struct {
			continue
		}
		keys := []editorSchemaKeyJSON{}
		editorCollectSchemaKeys("", f.Type, &keys, 0)
		if len(keys) == 0 {
			continue
		}
		out = append(out, editorSchemaSectionJSON{
			Section: tag,
			Title:   editorSectionTitle(tag),
			Field:   f.Name,
			Keys:    keys,
		})
	}
	return out
}

// editorCollectSchemaKeys flattens nested struct fields into dotted INI keys.
// Struct fields are namespaces (menu.item.font) unless the field is tagged with
// flatten="true", in which case its keys are merged into the parent namespace.
func editorCollectSchemaKeys(prefix string, t reflect.Type, out *[]editorSchemaKeyJSON, depth int) {
	if depth > 8 {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		tag := strings.TrimSpace(f.Tag.Get("ini"))
		if f.Anonymous && tag == "" {
			// Embedded structs share the namespace of their parent.
			editorCollectSchemaKeys(prefix, f.Type, out, depth+1)
			continue
		}
		if tag == "" || tag == "def" {
			continue
		}
		if f.Type.Kind() == reflect.Struct {
			if strings.EqualFold(strings.TrimSpace(f.Tag.Get("flatten")), "true") {
				editorCollectSchemaKeys(prefix, f.Type, out, depth+1)
			} else {
				editorCollectSchemaKeys(prefix+tag+".", f.Type, out, depth+1)
			}
			continue
		}
		if f.Type.Kind() == reflect.Map {
			// Map keys are user defined (e.g. menu.itemname.<name>). A
			// "map:<pattern>" tag carries the name inside the pattern, so the
			// lua tag names it (font) and otherwise the user name simply
			// continues at this level of the key path (cell.<name>).
			key := tag
			if strings.HasPrefix(key, "map:") {
				key = strings.TrimSpace(f.Tag.Get("lua"))
			} else if key == "" {
				key = strings.ToLower(f.Name)
			}
			name := prefix + key
			if name != "" && !strings.HasSuffix(name, ".") {
				name += "."
			}
			*out = append(*out, editorSchemaKeyJSON{
				Key:     name + "<name>",
				Type:    f.Type.String(),
				Default: strings.TrimSpace(f.Tag.Get("default")),
				Dynamic: true,
			})
			continue
		}
		*out = append(*out, editorSchemaKeyJSON{
			Key:     prefix + tag,
			Type:    f.Type.String(),
			Default: strings.TrimSpace(f.Tag.Get("default")),
		})
	}
}

// ---------------------------------------------------------------------------
// INI values of the configured motif / stage / character files
// ---------------------------------------------------------------------------

// editorKeyValueJSON is a single INI key and the value stored in the file.
type editorKeyValueJSON struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Choices []string `json:"choices,omitempty"`
}

// editorSectionJSON is a full INI section, as stored in a .def file.
type editorSectionJSON struct {
	Name  string               `json:"name"`
	Title string               `json:"title"`
	Keys  []editorKeyValueJSON `json:"keys"`
	// Raw holds the body of a section that has no keys of its own
	// ([Begin Action n], [Infobox Text], ...).
	Raw string `json:"raw,omitempty"`
}

// editorHeaderPattern matches an INI section header, capturing its name.
var editorHeaderPattern = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*([^\]]*?)[ \t]*\]`)

// editorRawSections returns, per normalized section name, the spelling the file
// uses and the section body. Sections such as [Begin Action n] or [Infobox Text]
// hold free form content that the INI parser does not turn into keys, so the
// body is kept for the editor to show.
func editorRawSections(text string) (headers map[string]string, bodies map[string]string) {
	headers = map[string]string{}
	bodies = map[string]string{}
	lines := strings.Split(NormalizeNewlines(text), "\n")
	name, norm := "", ""
	var body []string
	flush := func() {
		if norm == "" {
			return
		}
		// Drop the blank lines and the trailing comment block that surround a
		// section body: the comments belong to the header of the next section.
		start, end := 0, len(body)
		for end > start {
			t := strings.TrimSpace(body[end-1])
			if t == "" || strings.HasPrefix(t, ";") {
				end--
				continue
			}
			break
		}
		for start < end && strings.TrimSpace(body[start]) == "" {
			start++
		}
		bodies[norm] = strings.Join(body[start:end], "\n")
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// editorHeaderPattern is anchored and the line is already trimmed, so it
		// can only ever match when the line opens with '['. Gating on that cheap
		// test keeps the regex off the keys, comments and blank lines that make
		// up the bulk of a .def, which is ~12x faster on a 6k line motif.
		if strings.HasPrefix(trimmed, "[") {
			if m := editorHeaderPattern.FindStringSubmatch(trimmed); m != nil {
				flush()
				name = strings.TrimSpace(m[1])
				norm = editorNormSection(name)
				headers[norm] = name
				body = body[:0]
				continue
			}
		}
		if norm != "" {
			body = append(body, line)
		}
	}
	flush()
	return headers, bodies
}

// editorReadINISections reads every section of an INI file, preserving order.
// The size is checked first, so the helper is safe to call on any in-sandbox
// path.
func editorReadINISections(path string) ([]editorSectionJSON, error) {
	if _, err := editorFileWithinCap(path, editorMaxReadFile); err != nil {
		return nil, err
	}
	f, text, err := LoadINIFile(path, editorINIOptions())
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("no INI data in %v", path)
	}
	return editorSectionsFromINI(path, f, text), nil
}

// editorSectionsFromINI converts a parsed INI file into ordered sections.
func editorSectionsFromINI(path string, f *ini.File, text string) []editorSectionJSON {
	headers, bodies := editorRawSections(text)
	out := []editorSectionJSON{}
	for _, sec := range f.Sections() {
		name := sec.Name()
		if original, ok := headers[editorNormSection(name)]; ok {
			name = original
		}
		s := editorSectionJSON{Name: name, Title: name, Keys: []editorKeyValueJSON{}}
		if name == ini.DEFAULT_SECTION {
			s.Title = "(top level)"
		}
		for _, k := range sec.Keys() {
			if k.Name() == "" {
				continue
			}
			s.Keys = append(s.Keys, editorKeyValueJSON{
				Key:     k.Name(),
				Value:   k.Value(),
				Choices: editorChoicesFor(path, name, k.Name()),
			})
		}
		if len(s.Keys) == 0 {
			// Free form content ([Begin Action n], [Infobox Text], ...): keep the
			// body so the editor can show it.
			s.Raw = bodies[editorNormSection(name)]
		}
		out = append(out, s)
	}
	return out
}

// editorMotifINI parses the configured motif .def (user values only) and
// returns it together with the raw file text, so section names can be shown
// the way the file spells them.
func editorMotifINI() (*ini.File, string) {
	path := editorMotifPath()
	if path == "" {
		return nil, ""
	}
	if _, err := editorFileWithinCap(path, editorMaxReadFile); err != nil {
		LogMessage("[Editor] refusing to read motif %v: %v", path, err)
		return nil, ""
	}
	f, text, err := LoadINIFile(path, editorINIOptions())
	if err != nil {
		LogMessage("[Editor] unable to read motif %v: %v", path, err)
		return nil, ""
	}
	return f, text
}

// ---------------------------------------------------------------------------
// API handlers
// ---------------------------------------------------------------------------

// editorHandlePage serves the single page editor UI.
func editorHandlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, editorPageHTML)
}

// editorHandleStatus reports what the editor is currently looking at.
func editorHandleStatus(w http.ResponseWriter, r *http.Request) {
	motif := editorMotifPath()
	sel := editorSelectDefPath()
	exists := editorExistingFile(motif) != ""
	_, cmdFlag := sys.cmdFlags["-httpservice"]
	editorWriteJSON(w, map[string]any{
		"ok":                     true,
		"version":                Version,
		"buildTime":              BuildTime,
		"platform":               runtime.GOOS + "/" + runtime.GOARCH,
		"baseDir":                editorRootDir(),
		"port":                   EditorPort,
		"url":                    editorServiceBaseURL(),
		"motif":                  motif,
		"motifExists":            exists,
		"selectDef":              sel,
		"serviceFromCommandLine": cmdFlag,
		"structureSections":      len(editorMotifStructure()),
		"webview":                editorWebViewState(),
	})
}

// editorMotifKeyJSON is one motif key with the structural information the
// engine has about it (type + default) and its value in the motif file.
type editorMotifKeyJSON struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Default string   `json:"default,omitempty"`
	Type    string   `json:"type,omitempty"`
	Choices []string `json:"choices,omitempty"`
	Defined bool     `json:"defined"`
	Dynamic bool     `json:"dynamic,omitempty"`
	Unknown bool     `json:"unknown,omitempty"`
}

// editorMotifSectionJSON is one motif section, expanded structurally.
type editorMotifSectionJSON struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Field string `json:"field,omitempty"`
	// Runtime marks a section the engine parses outside the Motif struct (the
	// layers of a BGdef), where the file keys are valid as they are.
	Runtime bool                 `json:"runtime,omitempty"`
	Defined int                  `json:"defined"`
	Total   int                  `json:"total"`
	Keys    []editorMotifKeyJSON `json:"keys"`
	// Raw holds the body of a section that has no keys of its own
	// ([Begin Action n], [Infobox Text], ...).
	Raw string `json:"raw,omitempty"`
}

// editorMotifResponseJSON is the payload of /api/motif.
type editorMotifResponseJSON struct {
	Path      string                    `json:"path"`
	Exists    bool                      `json:"exists"`
	Sections  []editorMotifSectionJSON  `json:"sections"`
	Tree      []*editorGroupJSON        `json:"tree"`
	Structure []editorSchemaSectionJSON `json:"structure"`
}

// editorNormSection normalizes an INI section header for comparison with the
// engine's ini tags ("Title Info" -> "title_info").
func editorNormSection(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "_"))
}

// editorBuildSection expands one schema against the values of one file
// section: every schema key (fixed or dynamic) plus the file keys the schema
// does not know about.
func editorBuildSection(name, title, field string, sch *editorSchemaSectionJSON, vals map[string]string) editorMotifSectionJSON {
	section := editorMotifSectionJSON{
		Name: name, Title: title, Field: field, Keys: []editorMotifKeyJSON{},
	}
	var keys []editorSchemaKeyJSON
	if sch != nil {
		keys = sch.Keys
	}
	// Keys with a fixed name belong to their schema entry first, so a dynamic
	// entry can only claim what is left.
	seen := map[string]bool{}
	for _, sk := range keys {
		if !sk.Dynamic {
			seen[sk.Key] = true
		}
	}
	for _, sk := range keys {
		if sk.Dynamic {
			// Expand every user defined key matching this map entry.
			names := []string{}
			for k := range vals {
				if editorDynamicKeyMatch(sk.Key, k) {
					names = append(names, k)
				}
			}
			sort.Strings(names)
			for _, k := range names {
				if seen[k] {
					continue
				}
				seen[k] = true
				section.Keys = append(section.Keys, editorMotifKeyJSON{
					Key: k, Value: vals[k], Defined: true,
					Type: sk.Type, Dynamic: true,
					Choices: editorChoicesFor("", section.Name, k),
				})
			}
			continue
		}
		kv := editorMotifKeyJSON{
			Key: sk.Key, Type: sk.Type, Default: sk.Default,
			Choices: editorChoicesFor("", section.Name, sk.Key),
		}
		if v, ok := vals[sk.Key]; ok {
			kv.Value = v
			kv.Defined = true
		}
		section.Keys = append(section.Keys, kv)
	}
	// Keys present in the file that the engine does not know about.
	extra := []string{}
	for k := range vals {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		section.Keys = append(section.Keys, editorMotifKeyJSON{
			Key: k, Value: vals[k], Defined: true, Unknown: true,
			Choices: editorChoicesFor("", section.Name, k),
		})
	}
	// Group by state so the table can be scanned top down: what the file sets,
	// then what falls back to the engine default, then what the engine ignores.
	// Inside a group the order the schema declared is kept (a stable sort).
	sort.SliceStable(section.Keys, func(i, j int) bool {
		return editorKeyStateRank(section.Keys[i]) < editorKeyStateRank(section.Keys[j])
	})
	section.Total = len(section.Keys)
	for _, kv := range section.Keys {
		if kv.Defined {
			section.Defined++
		}
	}
	return section
}

// editorKeyStateRank orders the key states for display: defined, missing,
// unknown. The rank of a key is what decides the row order inside a section.
func editorKeyStateRank(k editorMotifKeyJSON) int {
	switch {
	case !k.Defined:
		return 1
	case k.Unknown:
		return 2
	default:
		return 0
	}
}

// editorBuildRuntimeSection expands a section the engine parses at load time
// instead of through the Motif struct: the layers of a <name>BGdef block and the
// background controllers. The keys come from the parser that reads them
// (readBackGround / bgCtrl.read in stage.go), so tooltips and
// the missing/unknown states are filled in exactly like a struct backed section.
func editorBuildRuntimeSection(name, title string, vals map[string]string) editorMotifSectionJSON {
	sch := editorRuntimeSchema(name)
	if sch == nil {
		// No schema to go by: list the keys the file has, as they are.
		names := []string{}
		for k := range vals {
			names = append(names, k)
		}
		sort.Strings(names)
		section := editorMotifSectionJSON{
			Name: name, Title: title, Runtime: true,
			Keys: []editorMotifKeyJSON{}, Total: len(names),
		}
		for _, k := range names {
			section.Keys = append(section.Keys, editorMotifKeyJSON{
				Key: k, Value: vals[k], Defined: true,
				Choices: editorChoicesFor("", name, k),
			})
			section.Defined++
		}
		return section
	}
	section := editorBuildSection(name, title, "", sch, vals)
	section.Runtime = true
	return section
}

// editorIsRuntimeSection reports whether a section belongs to a background
// definition of the motif: "[TitleBG Background Sky]", "[VersusBG StageInfo]",
// "[TitleBGctrl Fade]", ... Those are read by stage.go / bgdef.go when the motif
// is loaded. The plain "[BG n]" blocks of a stage .def are not part of a motif.
func editorIsRuntimeSection(name string) bool {
	return motifBGBlock(name) != ""
}

// editorMotifSections merges the parsed motif file with the structural layout
// from editorMotifStructure(), so the UI can show defined, missing and
// unrecognized keys for every section.
func editorMotifSections() []editorMotifSectionJSON {
	out := []editorMotifSectionJSON{}
	structure := editorMotifStructure()
	// Sections backed by the Motif struct. They are collected first and sorted by
	// the order of the file once every section is known.
	structural := []editorMotifSectionJSON{}

	fileValues := map[string]map[string]string{}
	fileOrder := []string{}
	headers := map[string]string{}
	rawBodies := map[string]string{}
	if f, text := editorMotifINI(); f != nil {
		_, bodies := editorRawSections(text)
		for _, sec := range editorSectionsFromINI("", f, text) {
			norm := editorNormSection(sec.Name)
			if _, dup := fileValues[norm]; dup {
				continue
			}
			keys := map[string]string{}
			for _, kv := range sec.Keys {
				keys[kv.Key] = kv.Value
			}
			fileValues[norm] = keys
			headers[norm] = sec.Name
			// Only a section without keys needs its body: with keys, the table
			// already shows everything the file has for it.
			if len(keys) == 0 {
				if body := firstNonEmpty(bodies[norm], sec.Raw); body != "" {
					rawBodies[norm] = body
				}
			}
			fileOrder = append(fileOrder, norm)
		}
	}

	used := map[string]bool{}
	// Sections that come from a map field of the Motif struct (results
	// screens, pause menus): the section name is only known from the file, so
	// the field pattern is matched against every header.
	mapSchemas := map[string]editorSchemaSectionJSON{}
	for _, sch := range structure {
		if sch.Kind != "map" || sch.Pattern == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + sch.Pattern)
		if err != nil {
			LogMessage("[Editor] invalid section pattern %q: %v", sch.Pattern, err)
			continue
		}
		for _, norm := range fileOrder {
			if re.MatchString(headers[norm]) {
				mapSchemas[norm] = sch
			}
		}
	}

	for _, sch := range structure {
		switch sch.Kind {
		case "map":
			// Handled below, once every file section is known.
			continue
		case "dynamic":
			// One fixed section with user named keys ([Music]).
			norm := editorNormSection(sch.Section)
			vals := fileValues[norm]
			if len(vals) == 0 {
				continue
			}
			used[norm] = true
			s := sch
			sec := editorBuildSection(headers[norm], sch.Title, sch.Field, &s, vals)
			sec.Raw = rawBodies[norm]
			structural = append(structural, sec)
			continue
		}
		norm := editorNormSection(sch.Section)
		vals := fileValues[norm]
		used[norm] = true
		name := headers[norm]
		if name == "" {
			name = sch.Title
		}
		s := sch
		sec := editorBuildSection(name, sch.Section, sch.Field, &s, vals)
		sec.Raw = rawBodies[norm]
		structural = append(structural, sec)
	}

	// The Motif struct does not declare the sections in the order a .def writes
	// them (Music comes before Info there), so follow the order of the file:
	// [Info], [Files], [Music], ... Sections the file does not have keep the
	// struct order and move to the end.
	fileIndex := map[string]int{}
	for i, norm := range fileOrder {
		fileIndex[norm] = i
	}
	filePos := func(s editorMotifSectionJSON) int {
		if i, ok := fileIndex[editorNormSection(s.Name)]; ok {
			return i
		}
		return len(fileOrder) + 1
	}
	sort.SliceStable(structural, func(i, j int) bool {
		return filePos(structural[i]) < filePos(structural[j])
	})
	out = append(out, structural...)

	// Sections declared by a map field, one entry per file section.
	for _, norm := range fileOrder {
		sch, ok := mapSchemas[norm]
		if !ok {
			continue
		}
		used[norm] = true
		s := sch
		sec := editorBuildSection(headers[norm], editorNormSection(headers[norm]), sch.Field, &s, fileValues[norm])
		sec.Raw = rawBodies[norm]
		out = append(out, sec)
	}

	// Sections of the file with no structural counterpart. Background layers
	// are parsed at runtime, so their keys are valid as they are; everything
	// else ([Begin Action ...], [Infobox Text], ...) is listed as unknown.
	for _, norm := range fileOrder {
		if used[norm] {
			continue
		}
		vals := fileValues[norm]
		name := headers[norm]
		if editorIsRuntimeSection(name) {
			runtimeSection := editorBuildRuntimeSection(name, name, vals)
			runtimeSection.Raw = rawBodies[norm]
			out = append(out, runtimeSection)
			continue
		}
		names := []string{}
		for k := range vals {
			names = append(names, k)
		}
		sort.Strings(names)
		section := editorMotifSectionJSON{
			Name:  name,
			Title: name,
			Keys:  []editorMotifKeyJSON{},
			Total: len(names),
			Raw:   rawBodies[norm],
		}
		for _, k := range names {
			section.Keys = append(section.Keys, editorMotifKeyJSON{
				Key: k, Value: vals[k], Defined: true, Unknown: true,
				Choices: editorChoicesFor("", section.Name, k),
			})
			section.Defined++
		}
		out = append(out, section)
	}
	return out
}

// ---------------------------------------------------------------------------
// Section tree: screens, background definitions and their actions
// ---------------------------------------------------------------------------

// editorSectionNodeJSON is one node of the motif section tree.
type editorSectionNodeJSON struct {
	Name     string                   `json:"name"`
	Title    string                   `json:"title,omitempty"`
	Field    string                   `json:"field,omitempty"`
	Kind     string                   `json:"kind,omitempty"`
	Runtime  bool                     `json:"runtime,omitempty"`
	Defined  int                      `json:"defined"`
	Total    int                      `json:"total"`
	Keys     []editorMotifKeyJSON     `json:"keys,omitempty"`
	Raw      string                   `json:"raw,omitempty"`
	Children []*editorSectionNodeJSON `json:"children,omitempty"`
}

// editorGroupJSON is a screen group of the tree.
type editorGroupJSON struct {
	Label    string                   `json:"label"`
	Sections []*editorSectionNodeJSON `json:"sections,omitempty"`
}

// editorScreenGroups orders the motif screens the way the Motif struct declares
// them, so the tree reads like the struct. A group without Tags is matched by
// section name instead (Results and Pause are map fields).
var editorScreenGroups = []struct {
	Label string
	Tags  []string
}{
	{Label: "Motif", Tags: []string{"info", "files", "music", "infobox", "glyphs"}},
	{Label: "Title", Tags: []string{"title_info", "titlebgdef"}},
	{Label: "Select", Tags: []string{"select_info", "selectbgdef"}},
	{Label: "Versus", Tags: []string{"vs_screen", "versusbgdef"}},
	{Label: "Continue", Tags: []string{"continue_screen", "continuebgdef"}},
	{Label: "Game Over", Tags: []string{"game_over_screen", "default_ending", "end_credits"}},
	{Label: "Victory", Tags: []string{"victory_screen", "victorybgdef"}},
	{Label: "Win", Tags: []string{"win_screen", "winbgdef"}},
	{Label: "Results"},
	{Label: "Option", Tags: []string{"option_info", "optionbgdef"}},
	{Label: "Replay", Tags: []string{"replay_info", "replaybgdef"}},
	{Label: "Pause"},
	{Label: "Attract", Tags: []string{"attract_mode", "attractbgdef"}},
	{Label: "Challenger", Tags: []string{"challenger_info", "challengerbgdef"}},
	{Label: "Dialogue", Tags: []string{"dialogue_info"}},
	{Label: "Hiscore", Tags: []string{"hiscore_info", "hiscorebgdef"}},
	{Label: "Warning", Tags: []string{"warning_info"}},
}

// firstNonEmpty returns the first non empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// editorSectionHead returns the first word of a section name, lowercased:
// "TitleBG Background Sky" -> "titlebg". The name is not normalized first,
// because normalization turns the spaces into underscores.
func editorSectionHead(name string) string {
	return editorFirstToken(strings.ToLower(strings.TrimSpace(name)))
}

// editorActionNo returns the number of a [Begin Action n] style section. The
// section name is normalized first, so "Begin Action 203" arrives here as
// "begin_action_203".
func editorActionNo(norm string) (int, bool) {
	for _, p := range []string{"begin_action_", "loop_action_", "end_action_", "beginaction", "loopaction", "endaction"} {
		if !strings.HasPrefix(norm, p) {
			continue
		}
		num := strings.Trim(strings.TrimPrefix(norm, p), "_ ")
		if n, err := strconv.Atoi(num); err == nil {
			return n, true
		}
	}
	return 0, false
}

// editorMotifTree nests the motif sections: one entry per screen, with the
// layers of every <name>BGdef below their background definition and each
// [Begin Action n] below the layer that plays it (actionno = n).
func editorMotifTree(sections []editorMotifSectionJSON) []*editorGroupJSON {
	groups := []*editorGroupJSON{}
	byLabel := map[string]*editorGroupJSON{}
	groupOf := func(label string) *editorGroupJSON {
		if g, ok := byLabel[label]; ok {
			return g
		}
		// Sections is always non nil, so a group without any section never
		// marshals to a null array.
		g := &editorGroupJSON{Label: label, Sections: []*editorSectionNodeJSON{}}
		byLabel[label] = g
		groups = append(groups, g)
		return g
	}
	for _, g := range editorScreenGroups {
		groupOf(g.Label)
	}
	other := groupOf("Other")

	// Which group a section belongs to, by tag or by name.
	groupFor := func(norm string) *editorGroupJSON {
		for _, g := range editorScreenGroups {
			for _, tag := range g.Tags {
				if norm == tag || strings.HasPrefix(norm, tag+"_") {
					return byLabel[g.Label]
				}
			}
		}
		if strings.Contains(norm, "results") {
			return byLabel["Results"]
		}
		if strings.Contains(norm, "pause") {
			return byLabel["Pause"]
		}
		return other
	}
	addTo := func(g *editorGroupJSON, n *editorSectionNodeJSON) {
		g.Sections = append(g.Sections, n)
	}

	// Node per section, keyed by its normalized INI header.
	nodes := map[string]*editorSectionNodeJSON{}
	for i := range sections {
		s := &sections[i]
		kind := "file"
		if s.Field != "" {
			kind = "screen"
		}
		nodes[editorNormSection(s.Name)] = &editorSectionNodeJSON{
			Name: s.Name, Title: s.Title, Field: s.Field, Kind: kind,
			Runtime: s.Runtime, Raw: s.Raw,
			Defined: s.Defined, Total: s.Total, Keys: s.Keys,
		}
	}

	// 1. Sections backed by the Motif struct, in screen order.
	for i := range sections {
		if sections[i].Field == "" {
			continue
		}
		addTo(groupFor(editorNormSection(sections[i].Title)), nodes[editorNormSection(sections[i].Name)])
	}

	// 2. Background layers and controllers, remembering the animation each
	//    layer plays.
	var lastBG *editorSectionNodeJSON
	layers := []*editorSectionNodeJSON{}
	for i := range sections {
		if sections[i].Field != "" {
			continue
		}
		norm := editorNormSection(sections[i].Name)
		head := motifBGBlock(sections[i].Name)
		if head == "" {
			// Not a motif background block ("[BG 0]" of a stage .def has no
			// <name>BGdef section of its own).
			continue
		}
		owner := nodes[head+"def"]
		if owner == nil {
			owner = &editorSectionNodeJSON{
				Name:  editorSectionTitle(head + "def"),
				Title: head + "def",
				Kind:  "bgdef",
			}
			nodes[head+"def"] = owner
			addTo(groupFor(head+"def"), owner)
		}
		node := nodes[norm]
		raw := editorSectionHead(sections[i].Name)
		if strings.HasSuffix(raw, "ctrl") || strings.HasSuffix(raw, "ctrldef") {
			node.Kind = "bgctrl"
			owner.Children = append(owner.Children, node)
			continue
		}
		node.Kind = "layer"
		owner.Children = append(owner.Children, node)
		layers = append(layers, node)
		lastBG = owner
	}
	playing := map[int]*editorSectionNodeJSON{}
	for _, node := range layers {
		for _, k := range node.Keys {
			if !strings.EqualFold(k.Key, "actionno") {
				continue
			}
			if num, err := strconv.Atoi(strings.TrimSpace(k.Value)); err == nil {
				if _, taken := playing[num]; !taken {
					playing[num] = node
				}
			}
		}
	}

	// 3. Actions, plus anything else the file contains.
	for i := range sections {
		if sections[i].Field != "" {
			continue
		}
		norm := editorNormSection(sections[i].Name)
		node := nodes[norm]
		num, isAction := editorActionNo(norm)
		switch {
		case isAction:
			node.Kind = "action"
			if owner, ok := playing[num]; ok {
				owner.Children = append(owner.Children, node)
			} else if lastBG != nil {
				lastBG.Children = append(lastBG.Children, node)
			} else {
				addTo(groupFor(norm), node)
			}
		case motifBGBlock(sections[i].Name) != "":
			// A background layer or controller: already nested under its BGdef.
		default:
			addTo(groupFor(norm), node)
		}
	}
	return groups
}

// editorHandleMotif serves the structural breakdown of the current motif.
func editorHandleMotif(w http.ResponseWriter, r *http.Request) {
	path := editorMotifPath()
	exists := editorExistingFile(path) != ""
	if !exists {
		LogMessage("[Editor] motif not found: %v", path)
	}
	sections := editorMotifSections()
	editorWriteJSON(w, editorMotifResponseJSON{
		Path:      path,
		Exists:    exists,
		Sections:  sections,
		Tree:      editorMotifTree(sections),
		Structure: editorMotifStructure(),
	})
}

// ---------------------------------------------------------------------------
// Stage / character breakdown (select.def)
// ---------------------------------------------------------------------------

// editorInfoJSON is the [Info] block of a character or stage def.
type editorInfoJSON struct {
	Name       string `json:"name,omitempty"`
	Author     string `json:"author,omitempty"`
	Localcoord string `json:"localcoord,omitempty"`
}

// editorCharacterEntryJSON is one line of the select.def [Characters] section.
type editorCharacterEntryJSON struct {
	Def       string            `json:"def"`
	Path      string            `json:"path,omitempty"`
	Stages    []string          `json:"stages,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	Available bool              `json:"available"`
	Info      editorInfoJSON    `json:"info,omitempty"`
}

// editorStageEntryJSON is one line of the select.def [ExtraStages] section.
type editorStageEntryJSON struct {
	Name      string            `json:"name"`
	Def       string            `json:"def"`
	Path      string            `json:"path,omitempty"`
	Source    string            `json:"source"`
	Params    map[string]string `json:"params,omitempty"`
	Available bool              `json:"available"`
	Info      editorInfoJSON    `json:"info,omitempty"`
}

// editorExistingFile returns the path when it points to an existing file.
func editorExistingFile(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	abs := filepath.FromSlash(p)
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return filepath.ToSlash(abs)
	}
	return ""
}

// editorFileWithinCap reports the size of the existing file at p, refusing it
// when it is larger than max. The size is stat'ed before anything is read, so a
// multi gigabyte in-sandbox blob (a log, a movie, an archive) is rejected
// rather than loaded into memory.
func editorFileWithinCap(p string, max int64) (int64, error) {
	info, err := os.Stat(filepath.FromSlash(p))
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("not a file: %v", p)
	}
	if info.Size() > max {
		return 0, fmt.Errorf("file is too large to read (%d bytes, limit %d): %v", info.Size(), max, p)
	}
	return info.Size(), nil
}

// editorTextExtensions are the file types the editor reads as text through
// /api/file and the INI helpers. Restricting the extension keeps the endpoint
// from parsing arbitrary binaries as INI and from reading huge media files.
var editorTextExtensions = map[string]bool{
	".def": true, ".ini": true, ".txt": true, ".cfg": true,
	".air": true, ".cmd": true, ".cns": true, ".st": true,
	".json": true, ".lua": true, ".md": true,
}

// editorReadableTextFile resolves p inside the game folder and checks it is an
// existing text-like file within editorMaxReadFile, returning its absolute
// path. It is the single gate for every whole-file read of the editor.
func editorReadableTextFile(p string) (string, error) {
	abs, err := editorSandboxPath(p)
	if err != nil {
		return "", err
	}
	existing := editorExistingFile(abs)
	if existing == "" {
		return "", fmt.Errorf("file not found: %v", p)
	}
	if !editorTextExtensions[strings.ToLower(filepath.Ext(existing))] {
		return "", fmt.Errorf("unsupported file type: %v", filepath.Base(existing))
	}
	if _, err := editorFileWithinCap(existing, editorMaxReadFile); err != nil {
		return "", err
	}
	return filepath.FromSlash(existing), nil
}

// editorFileReadStatus maps an editorReadableTextFile error to the HTTP status
// the API reports it with.
func editorFileReadStatus(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	case strings.Contains(msg, "too large"):
		return http.StatusRequestEntityTooLarge
	case strings.Contains(msg, "unsupported file type"):
		return http.StatusUnsupportedMediaType
	default:
		return http.StatusBadRequest
	}
}

// editorResolveDefPath resolves a character/stage reference from select.def to a
// relative path inside the game folder (empty when it cannot be found).
func editorResolveDefPath(def, dir string) string {
	def = strings.TrimSpace(filepath.ToSlash(def))
	if def == "" {
		return ""
	}
	base := def
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	cands := []string{def, dir + "/" + def}
	if filepath.Ext(def) == "" {
		cands = append(cands,
			def+".def", dir+"/"+def+".def",
			def+"/"+base+".def", dir+"/"+def+"/"+base+".def")
	}
	for _, c := range cands {
		if _, err := editorSandboxPath(c); err != nil {
			continue
		}
		if editorExistingFile(c) != "" {
			return filepath.ToSlash(filepath.Clean(filepath.FromSlash(c)))
		}
	}
	return ""
}

// editorUnquote strips the surrounding quotes the INI loader preserves.
func editorUnquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// editorInfoCacheMax bounds the [Info] cache. The entries are a few dozen bytes
// each, so the cap only exists to keep a long session that browses many .def
// files from growing without bound.
const editorInfoCacheMax = 256

// editorInfoEntry is one cached [Info] block plus the identity it was read
// from. Like the .sff cache, the mtime and size are re-checked on every lookup,
// so editing a .def on disk invalidates the entry instead of serving stale
// metadata.
type editorInfoEntry struct {
	info  editorInfoJSON
	mtime time.Time
	size  int64
	used  int64
}

// editorInfoCache memoises editorReadInfo. It is called once per select.def
// entry and once per file found in the stages/ scan, so a /api/characters or
// /api/stages request otherwise re-reads and re-parses every def it touches --
// measured at ~13.7 ms and ~3 MB of garbage per call on a 6k line .def, i.e.
// over a second and ~300 MB for a 100 character roster. Repeated requests then
// re-paid it again. Entries are validated against the file and evicted LRU.
var editorInfoCache = struct {
	mu   sync.Mutex
	m    map[string]*editorInfoEntry
	tick int64
}{m: map[string]*editorInfoEntry{}}

// editorInfoCacheLookup returns the cached [Info] for path when it still
// matches the file on disk, dropping it otherwise.
func editorInfoCacheLookup(path string) *editorInfoJSON {
	mtime, size, err := editorFileStat(path)
	if err != nil {
		return nil
	}
	editorInfoCache.mu.Lock()
	defer editorInfoCache.mu.Unlock()
	e := editorInfoCache.m[path]
	if e == nil {
		return nil
	}
	if e.size != size || !e.mtime.Equal(mtime) {
		delete(editorInfoCache.m, path)
		return nil
	}
	editorInfoCache.tick++
	e.used = editorInfoCache.tick
	return &e.info
}

// editorInfoCachePut stores one [Info] block and evicts the least recently
// used entry once the cache is over its cap.
func editorInfoCachePut(path string, info editorInfoJSON) {
	mtime, size, err := editorFileStat(path)
	if err != nil {
		return
	}
	editorInfoCache.mu.Lock()
	defer editorInfoCache.mu.Unlock()
	editorInfoCache.tick++
	editorInfoCache.m[path] = &editorInfoEntry{
		info: info, mtime: mtime, size: size, used: editorInfoCache.tick,
	}
	for len(editorInfoCache.m) > editorInfoCacheMax {
		var oldest string
		var oldestUsed int64
		for k, e := range editorInfoCache.m {
			if oldest == "" || e.used < oldestUsed {
				oldest, oldestUsed = k, e.used
			}
		}
		delete(editorInfoCache.m, oldest)
	}
}

// editorReadInfo reads the [Info] block of a def file (best effort).
//
// Only a successful parse is cached, so a missing, unreadable or [Info]-less
// file keeps returning the zero value until it changes, exactly as before.
func editorReadInfo(path string) editorInfoJSON {
	info := editorInfoJSON{}
	if path == "" {
		return info
	}
	if cached := editorInfoCacheLookup(path); cached != nil {
		return *cached
	}
	if _, err := editorFileWithinCap(path, editorMaxReadFile); err != nil {
		return info
	}
	f, _, err := LoadINIFile(filepath.FromSlash(path), editorINIOptions())
	if err != nil || f == nil {
		return info
	}
	sec, err := f.GetSection("Info")
	if err != nil || sec == nil {
		return info
	}
	info.Name = editorUnquote(editorINIKeyValue(sec, "name"))
	info.Author = editorUnquote(editorINIKeyValue(sec, "author"))
	info.Localcoord = editorUnquote(editorINIKeyValue(sec, "localcoord"))
	editorInfoCachePut(path, info)
	return info
}

// editorSelectDefSections mirrors the section handling of external/script/main.lua.
func editorSelectDefSections(text string) (characters []editorCharacterEntryJSON, stages []editorStageEntryJSON) {
	characters = []editorCharacterEntryJSON{}
	stages = []editorStageEntryJSON{}
	section := 0
	for _, raw := range strings.Split(NormalizeNewlines(text), "\n") {
		line := raw
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			name := strings.ToLower(strings.Trim(strings.TrimSpace(line), "[]"))
			// Language prefixed sections ([en.characters]) map onto the base name.
			if i := strings.Index(name, "."); i >= 0 {
				name = name[i+1:]
			}
			switch strings.TrimSpace(name) {
			case "characters":
				section = 1
			case "extrastages":
				section = 2
			default:
				section = -1
			}
			continue
		}
		if section != 1 && section != 2 {
			continue
		}
		parts := strings.Split(line, ",")
		def := strings.TrimSpace(parts[0])
		if def == "" {
			continue
		}
		params := map[string]string{}
		plain := []string{}
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if k, v, found := strings.Cut(p, "="); found {
				params[strings.TrimSpace(k)] = strings.TrimSpace(v)
			} else {
				plain = append(plain, p)
			}
		}
		if section == 1 {
			e := editorCharacterEntryJSON{Def: def, Params: params, Stages: plain}
			e.Path = editorResolveDefPath(def, "chars")
			e.Available = e.Path != ""
			e.Info = editorReadInfo(e.Path)
			characters = append(characters, e)
		} else {
			e := editorStageEntryJSON{Name: def, Def: def, Source: "select.def", Params: params}
			if len(plain) > 0 {
				// Extra stages may list several defs, all sharing the params.
				e.Def = plain[0]
			}
			e.Path = editorResolveDefPath(e.Def, "stages")
			e.Available = e.Path != ""
			e.Info = editorReadInfo(e.Path)
			stages = append(stages, e)
		}
	}
	return characters, stages
}

// editorListDefFiles returns *.def files inside a folder (shallow scan).
func editorListDefFiles(dir string, limit int) []string {
	out := []string{}
	abs, err := editorSandboxPath(dir)
	if err != nil {
		return out
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return out
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".def") {
			continue
		}
		names = append(names, filepath.ToSlash(filepath.Join(filepath.FromSlash(dir), e.Name())))
	}
	sort.Strings(names)
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	return append(out, names...)
}

// editorHandleStages serves the stage list built from select.def.
func editorHandleStages(w http.ResponseWriter, r *http.Request) {
	sel := editorSelectDefPath()
	if _, err := editorFileWithinCap(sel, editorMaxReadFile); err != nil {
		editorWriteError(w, editorFileReadStatus(err), "unable to read %v: %v", sel, err)
		return
	}
	text, err := LoadText(sel)
	if err != nil {
		editorWriteError(w, http.StatusNotFound, "unable to read %v: %v", sel, err)
		return
	}
	_, stages := editorSelectDefSections(text)
	for _, f := range editorListDefFiles("stages", 500) {
		found := false
		for _, s := range stages {
			if strings.EqualFold(s.Path, f) || strings.EqualFold(s.Def, f) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		stages = append(stages, editorStageEntryJSON{
			Name:      filepath.Base(filepath.FromSlash(f)),
			Def:       f,
			Path:      f,
			Source:    "stages/",
			Available: true,
			Info:      editorReadInfo(f),
		})
	}
	editorWriteJSON(w, map[string]any{"ok": true, "selectDef": sel, "stages": stages})
}

// editorHandleFile serves the INI sections of any file inside the game folder.
func editorHandleFile(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		rel = editorMotifPath()
	}
	abs, err := editorReadableTextFile(rel)
	if err != nil {
		editorWriteError(w, editorFileReadStatus(err), "%v", err)
		return
	}
	sections, err := editorReadINISections(abs)
	if err != nil {
		editorWriteError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	editorWriteJSON(w, map[string]any{
		"ok":       true,
		"path":     filepath.ToSlash(abs),
		"sections": sections,
	})
}

// ---------------------------------------------------------------------------
// Sprite preview (/api/sff)
// ---------------------------------------------------------------------------

// editorSffCacheMax is how many decoded .sff files are kept at once. A decoded
// file holds its pixel data for the process lifetime, so the cache is bounded
// and evicts the least recently used entry past this.
const editorSffCacheMax = 8

// editorSffEntry is one decoded .sff plus the on-disk identity it was read
// from. The mtime and size are checked on every lookup, so editing a .sff on
// disk invalidates the entry instead of serving stale sprites.
type editorSffEntry struct {
	sff   *Sff
	mtime time.Time
	size  int64
	used  int64
	// png caches the encoded PNG of every sprite served so far, so paging
	// back to a sprite does not re-encode it. The entry is rebuilt when the
	// file changes, which drops the PNGs with it.
	png map[[2]uint16][]byte
}

// editorSffCache holds the .sff files the editor decoded for preview, so paging
// through sprites does not re-read the whole file every time. Entries are
// invalidated when the file changes and evicted LRU once the cap is reached.
var editorSffCache = struct {
	mu   sync.Mutex
	m    map[string]*editorSffEntry
	tick int64
}{m: map[string]*editorSffEntry{}}

// editorFileStat reads the identity a cache entry is validated against.
func editorFileStat(abs string) (time.Time, int64, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return time.Time{}, 0, err
	}
	return info.ModTime(), info.Size(), nil
}

// editorSffCacheLookup returns the cached entry when it still matches the file
// on disk, dropping it otherwise.
func editorSffCacheLookup(abs string) *editorSffEntry {
	mtime, size, err := editorFileStat(abs)
	if err != nil {
		return nil
	}
	editorSffCache.mu.Lock()
	defer editorSffCache.mu.Unlock()
	e := editorSffCache.m[abs]
	if e == nil {
		return nil
	}
	if e.size != size || !e.mtime.Equal(mtime) {
		delete(editorSffCache.m, abs)
		return nil
	}
	editorSffCache.tick++
	e.used = editorSffCache.tick
	return e
}

// editorSffCachePut stores a decoded .sff and evicts the least recently used
// entry when the cache is over its cap.
func editorSffCachePut(abs string, s *Sff) {
	mtime, size, err := editorFileStat(abs)
	if err != nil {
		return
	}
	editorSffCache.mu.Lock()
	defer editorSffCache.mu.Unlock()
	editorSffCache.tick++
	editorSffCache.m[abs] = &editorSffEntry{
		sff: s, mtime: mtime, size: size, used: editorSffCache.tick,
		png: map[[2]uint16][]byte{},
	}
	for len(editorSffCache.m) > editorSffCacheMax {
		var oldest string
		var oldestUsed int64
		for k, e := range editorSffCache.m {
			if oldest == "" || e.used < oldestUsed {
				oldest, oldestUsed = k, e.used
			}
		}
		delete(editorSffCache.m, oldest)
	}
}

// editorLoadSff returns the decoded .sff at an absolute path, loading it on
// first use and reloading it when the file on disk has changed.
func editorLoadSff(abs string) (*Sff, error) {
	e, err := editorSffCacheEntry(abs)
	if err != nil {
		return nil, err
	}
	return e.sff, nil
}

// editorSffCacheEntry returns the live cache entry for abs, loading the file
// when it is missing or stale.
func editorSffCacheEntry(abs string) (*editorSffEntry, error) {
	if e := editorSffCacheLookup(abs); e != nil {
		return e, nil
	}
	if _, err := editorFileWithinCap(abs, editorMaxSFF); err != nil {
		return nil, err
	}
	s, err := loadSffEx(abs, true, false, false, true)
	if err != nil {
		return nil, err
	}
	editorSffCachePut(abs, s)
	e := editorSffCacheLookup(abs)
	if e == nil {
		// The file changed between the load and the lookup: fall back to the
		// freshly decoded copy without caching it.
		return &editorSffEntry{sff: s, png: map[[2]uint16][]byte{}}, nil
	}
	return e, nil
}

// editorSffSpritePNG returns the encoded PNG of one sprite, encoding it on
// first use and caching the bytes on the entry.
func editorSffSpritePNG(e *editorSffEntry, key [2]uint16, spr *Sprite) ([]byte, error) {
	editorSffCache.mu.Lock()
	if e.png == nil {
		e.png = map[[2]uint16][]byte{}
	}
	if b := e.png[key]; b != nil {
		editorSffCache.mu.Unlock()
		return b, nil
	}
	editorSffCache.mu.Unlock()
	img := editorSffSpriteImage(e.sff, spr)
	if img == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	editorSffCache.mu.Lock()
	e.png[key] = b
	editorSffCache.mu.Unlock()
	return b, nil
}

// editorSffSpriteImage renders one sprite to an image. 8 bit sprites are
// paletted, so they go through the sprite's palette; 24 and 32 bit sprites
// carry their own colors. Returns nil when the sprite has no readable pixels.
func editorSffSpriteImage(sff *Sff, spr *Sprite) *image.RGBA {
	if sff == nil || spr == nil || len(spr.pendingData) == 0 {
		return nil
	}
	w, h := int(spr.pendingW), int(spr.pendingH)
	if w <= 0 || h <= 0 {
		return nil
	}
	bpp := int(spr.pendingDepth) / 8
	if bpp < 1 {
		bpp = 1
	}
	if len(spr.pendingData) < w*h*bpp {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	switch spr.pendingDepth {
	case 8:
		// Palette colors are packed 0xAABBGGRR, matching the engine.
		pal := spr.GetPal(&sff.palList)
		if len(pal) == 0 {
			return nil
		}
		for y := 0; y < h; y++ {
			row := spr.pendingData[y*w : (y+1)*w]
			o := y * img.Stride
			for x, idx := range row {
				if int(idx) >= len(pal) {
					continue
				}
				c := pal[idx]
				img.Pix[o+x*4+0] = byte(c)
				img.Pix[o+x*4+1] = byte(c >> 8)
				img.Pix[o+x*4+2] = byte(c >> 16)
				img.Pix[o+x*4+3] = byte(c >> 24)
			}
		}
	case 24:
		for y := 0; y < h; y++ {
			copy(img.Pix[y*img.Stride:y*img.Stride+w*3], spr.pendingData[y*w*3:(y+1)*w*3])
			for x := 0; x < w; x++ {
				img.Pix[y*img.Stride+x*3+3] = 0xff
			}
		}
	case 32:
		for y := 0; y < h; y++ {
			copy(img.Pix[y*img.Stride:(y+1)*img.Stride], spr.pendingData[y*w*4:(y+1)*w*4])
		}
	default:
		return nil
	}
	return img
}

// editorHandleSFF serves one sprite of a .sff as a PNG, so the page can show it
// in a plain <img> tag. Parameters: file=chars/kfm/kfm.sff group=9000 number=0.
func editorHandleSFF(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rel := strings.TrimSpace(q.Get("file"))
	if rel == "" {
		editorWriteError(w, http.StatusBadRequest, "missing file parameter")
		return
	}
	group, err := editorSffIndex(q.Get("group"), "group")
	if err != nil {
		editorWriteError(w, http.StatusBadRequest, "%v", err)
		return
	}
	number, err := editorSffIndex(q.Get("number"), "number")
	if err != nil {
		editorWriteError(w, http.StatusBadRequest, "%v", err)
		return
	}
	abs, err := editorSandboxPath(rel)
	if err != nil {
		editorWriteError(w, http.StatusBadRequest, "%v", err)
		return
	}
	if ext := strings.ToLower(filepath.Ext(abs)); ext != ".sff" {
		editorWriteError(w, http.StatusBadRequest, "not a .sff file: %v", rel)
		return
	}
	abs = editorExistingFile(abs)
	if abs == "" {
		editorWriteError(w, http.StatusNotFound, "file not found: %v", rel)
		return
	}
	entry, err := editorSffCacheEntry(abs)
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "too large") {
			code = http.StatusRequestEntityTooLarge
		}
		editorWriteError(w, code, "unable to read %v: %v", rel, err)
		return
	}
	key := [2]uint16{group, number}
	spr := entry.sff.sprites[key]
	if spr == nil {
		editorWriteError(w, http.StatusNotFound, "no sprite %v,%v in %v", group, number, rel)
		return
	}
	// PNG is in the standard library and every browser renders it in an <img>.
	// The encoded bytes are cached, so paging back to a sprite is free.
	data, err := editorSffSpritePNG(entry, key, spr)
	if err != nil {
		editorWriteError(w, http.StatusInternalServerError, "unable to encode %v,%v of %v: %v", group, number, rel, err)
		return
	}
	if data == nil {
		editorWriteError(w, http.StatusNotFound, "sprite %v,%v of %v has no image data", group, number, rel)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// editorSffIndex parses a sprite coordinate (group or number) as a uint16.
func editorSffIndex(s, name string) (uint16, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("missing %v parameter", name)
	}
	v, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid %v parameter: %v", name, s)
	}
	return uint16(v), nil
}

// ---------------------------------------------------------------------------
// Live apply (motif only)
// ---------------------------------------------------------------------------

// editorApplyTimeout bounds how long a save waits for the engine to pick the
// change up. System.await drains the main thread task queue once per frame in
// every game state, so one frame is enough; the rest is slack.
const editorApplyTimeout = 2 * time.Second

// editorFontKeyRe matches the [Files] keys that describe one of the fonts the
// engine loads (FilesProperties.Font is keyed "font0", "font1", ...).
var editorFontKeyRe = regexp.MustCompile(`^font[0-9]+\.(file|height)$`)

// editorRunOnMainThread runs fn on the engine thread and waits for its result.
// The game loop reads the motif every frame, so its fields must never be
// written from the HTTP goroutine. System.await calls runMainThreadTask every
// frame, which is what makes this safe to post to.
func editorRunOnMainThread(fn func() error) error {
	done := make(chan error, 1)
	select {
	case sys.mainThreadTask <- func() { done <- fn() }:
	default:
		return fmt.Errorf("the engine task queue is full")
	}
	select {
	case err := <-done:
		return err
	case <-time.After(editorApplyTimeout):
		return fmt.Errorf("the engine did not apply the change within %v", editorApplyTimeout)
	}
}

// editorApplyOutcome is what a live apply decides on the engine thread and
// reports back to the HTTP goroutine.
type editorApplyOutcome struct {
	applied bool
	reason  string
	warning string
	err     error
}

// editorRunOnEngineThread posts a job to the engine thread and waits for its
// outcome. It is editorRunOnMainThread generalized to work that has a result:
// classification reads the live motif, and every field of it is only ever read
// and written on the engine thread.
func editorRunOnEngineThread(fn func() editorApplyOutcome) (editorApplyOutcome, error) {
	return editorRunOnEngineThreadTimeout(editorApplyTimeout, fn)
}

// editorRunOnEngineThreadTimeout is editorRunOnEngineThread with an explicit
// wait bound. A motif reload re-parses the file and rebuilds its assets, so it
// waits longer than a single key apply.
func editorRunOnEngineThreadTimeout(timeout time.Duration, fn func() editorApplyOutcome) (editorApplyOutcome, error) {
	done := make(chan editorApplyOutcome, 1)
	select {
	case sys.mainThreadTask <- func() { done <- fn() }:
	default:
		return editorApplyOutcome{}, fmt.Errorf("the engine task queue is full")
	}
	select {
	case out := <-done:
		return out, nil
	case <-time.After(timeout):
		return editorApplyOutcome{}, fmt.Errorf("the engine did not apply the change within %v", timeout)
	}
}

// The load time pointers the Lua script holds a handle to: toLValue turns each
// of them into userdata wrapping the Go pointer, so an object handed to the
// script must be updated in place rather than replaced.
var (
	editorTextSpriteType = reflect.TypeOf((*TextSprite)(nil))
	editorPalFxType      = reflect.TypeOf((*PalFX)(nil))
	editorRectType       = reflect.TypeOf((*Rect)(nil))
	editorFadeType       = reflect.TypeOf((*Fade)(nil))
	editorAnimType       = reflect.TypeOf((*Anim)(nil))
)

// editorDerivedPtrs are the pointer types PopulateDataPointers builds at load
// time. A struct that declares one of them has part of itself snapshotted, so
// the draw path reads the snapshot rather than the field.
var editorDerivedPtrs = []reflect.Type{
	editorTextSpriteType, editorPalFxType, editorRectType, editorFadeType, editorAnimType,
}

// editorMotifKeyDerivedPtrs walks a query through the Motif struct the way the
// loader does and returns the load time pointers the struct declaring the field
// owns. A non empty result means the value is drawn from a snapshot rather than
// read from the field.
func editorMotifKeyDerivedPtrs(query string) []reflect.Type {
	parts := parseQueryPath(query)
	if len(parts) < 2 {
		return editorDerivedPtrs
	}
	owner := reflect.TypeOf(Motif{})
	for i := 0; i < len(parts); i++ {
		for owner.Kind() == reflect.Ptr {
			owner = owner.Elem()
		}
		if owner.Kind() != reflect.Struct {
			return editorDerivedPtrs
		}
		declStruct, fieldType, ok := editorDeclaringField(owner, parts[i].name)
		if !ok {
			// The engine does not model this name, so nothing can apply it.
			return editorDerivedPtrs
		}
		if i == len(parts)-1 {
			return editorDeclaredDerivedPtrs(declStruct)
		}
		if fieldType.Kind() == reflect.Map {
			if i+2 < len(parts) {
				owner = fieldType.Elem()
				i++ // the map key
				continue
			}
			// Default-key form: the map holds a single implicit entry and
			// the next part names a field of the element
			// (p1.cursor.active.scale, p1.cursor.done.snd, ... for p1-p8).
			// Descend without skipping so the Anim-only element classifies
			// as reload instead of refresh.
			if i+1 < len(parts) {
				elem := fieldType.Elem()
				for elem.Kind() == reflect.Ptr {
					elem = elem.Elem()
				}
				if elem.Kind() == reflect.Struct {
					if _, _, ok := editorDeclaringField(elem, parts[i+1].name); ok {
						owner = fieldType.Elem()
						continue
					}
				}
			}
			return editorDerivedPtrs
		}
		owner = fieldType
	}
	return editorDerivedPtrs
}

// editorDeclaredDerivedPtrs returns the load time pointers t itself declares.
func editorDeclaredDerivedPtrs(t reflect.Type) []reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var found []reflect.Type
	for i := 0; i < t.NumField(); i++ {
		for _, want := range editorDerivedPtrs {
			if t.Field(i).Type == want {
				found = append(found, want)
			}
		}
	}
	return found
}

// editorDeclaringField returns the struct that declares the field carrying the
// given ini tag, plus the field's own type, looking into anonymous embedded
// structs the way the engine does.
func editorDeclaringField(t reflect.Type, tag string) (reflect.Type, reflect.Type, bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, nil, false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if it := f.Tag.Get("ini"); it != "" && strings.EqualFold(it, tag) {
			return t, f.Type, true
		}
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" || !f.Anonymous {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			if ds, fp, ok := editorDeclaringField(ft, tag); ok {
				return ds, fp, true
			}
		}
	}
	return nil, nil, false
}

// editorFillSnapshot applies the owning struct's values to one of the load time
// pointers under v, in place. The object is not replaced: the Lua script was
// handed a handle to it, and that handle stays valid only for the object it was
// given.
func editorFillSnapshot(root *Motif, f, v, parent reflect.Value) {
	switch f.Type() {
	case editorTextSpriteType:
		if p, _ := f.Interface().(*TextSprite); p != nil {
			setTextSpriteInto(p, root, f, v, parent)
		}
	case editorPalFxType:
		if p, _ := f.Interface().(*PalFX); p != nil {
			setPalFxInto(p, root, f, v, parent)
		}
	case editorRectType:
		if p, _ := f.Interface().(*Rect); p != nil {
			setRectInto(p, root, f, v, parent)
		}
	case editorFadeType:
		if p, _ := f.Interface().(*Fade); p != nil {
			setFadeInto(p, root, f, v, parent)
		}
	}
}

// editorRefillSnapshots walks the structs under v and refills every snapshot it
// finds from the values they now hold.
func editorRefillSnapshots(root *Motif, v, parent reflect.Value) {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		fillable := false
		for _, want := range editorDerivedPtrs {
			if f.Type() == want {
				editorFillSnapshot(root, f, v, parent)
				fillable = true
				break
			}
		}
		if !fillable && f.Kind() == reflect.Struct {
			editorRefillSnapshots(root, f, v)
		}
	}
}

// editorRefreshMotifScope returns the top level Motif field a query lives under,
// which is the screen being edited: the unit of work worth rebuilding. It is nil
// when the key sits under a map ([Survival Results Screen], [Pause Menu],
// [Files] fontN), where there is no single field to scope to.
func editorRefreshMotifScope(m *Motif, query string) reflect.Value {
	parts := parseQueryPath(query)
	if m == nil || len(parts) == 0 {
		return reflect.Value{}
	}
	fv, _, ok := findFieldByINITag(reflect.ValueOf(m).Elem(), parts[0].name)
	if !ok {
		return reflect.Value{}
	}
	return fv
}

// editorHasPopulatedSnapshot reports whether anything under v was built at load
// time. applyPostParsePosAdjustments dereferences every snapshot it walks, so it
// is only safe on a motif that has been through PopulateDataPointers.
func editorHasPopulatedSnapshot(v reflect.Value) bool {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Map {
		// A map of snapshots: any non-nil entry counts.
		iter := v.MapRange()
		for iter.Next() {
			e := iter.Value()
			for e.Kind() == reflect.Ptr || e.Kind() == reflect.Interface {
				if e.IsNil() {
					break
				}
				e = e.Elem()
			}
			if e.Kind() == reflect.Struct && editorHasPopulatedSnapshot(e) {
				return true
			}
		}
		return false
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		for _, want := range editorDerivedPtrs {
			if f.Type() == want {
				return !f.IsNil()
			}
		}
		if f.Kind() == reflect.Struct && editorHasPopulatedSnapshot(f) {
			return true
		}
	}
	return false
}

// editorFieldByQuery returns the value a query names inside m, the same walk the
// loader does. Maps are not followed: a user named section has no single field
// to address.
func editorFieldByQuery(m *Motif, query string) (reflect.Value, bool) {
	parts := parseQueryPath(query)
	if len(parts) < 2 {
		return reflect.Value{}, false
	}
	v := reflect.ValueOf(m).Elem()
	for i := 0; i < len(parts); i++ {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		fv, _, ok := findFieldByINITag(v, parts[i].name)
		if !ok {
			return reflect.Value{}, false
		}
		if i == len(parts)-1 {
			return fv, true
		}
		v = fv
	}
	return reflect.Value{}, false
}

// editorSyncMotifLuaTable rewrites one key of the motif table the Lua script is
// already holding. The script reads plain values (menu.tween.factor,
// menu.item.spacing, ...) straight out of that table, so assigning the Go field
// alone is invisible to it. The snapshotted pointers need no sync: they are
// userdata wrapping the objects refilled in place.
//
// It only replaces an existing entry of the cached table and runs no Lua code,
// so it is safe on the engine thread between frames.
func editorSyncMotifLuaTable(m *Motif, query string) {
	l := sys.luaLState
	if l == nil || sys.cachedMotifTable == nil {
		// The script has not built its motif table yet (or the motif was never
		// loaded through Lua); there is nothing to keep in sync.
		return
	}
	parts := parseQueryPath(query)
	if len(parts) < 2 {
		return
	}
	field, ok := editorFieldByQuery(m, query)
	if !ok {
		return
	}
	cur := sys.cachedMotifTable
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur.RawGetString(p.name).(*lua.LTable)
		if !ok {
			return
		}
		cur = next
	}
	last := parts[len(parts)-1].name
	if cur.RawGetString(last) == lua.LNil {
		// Not a key the table has: leave the shape alone.
		return
	}
	cur.RawSetString(last, toLValue(l, field.Interface()))
}

// editorReapplyMotifScreen recomputes the screen a key belongs to, so an edit to
// a value the load time passes derived from becomes visible. It must run on the
// engine thread, after the field has been assigned.
//
// Nothing here replaces a snapshot: the Lua script was handed a handle to each
// one, and a handle is only valid for the object it was given, so they are all
// refilled in place. That is also why the order matters. setTextSpriteInto ends
// by calling SetPos with the struct's own Offset, which restores the pristine
// offsetInit, and only then does applyPostParsePosAdjustments add the container
// offsets (Menu.Pos and friends) on top. Running the position pass on its own
// would add the container offset to an offsetInit that already contains it, and
// the sprite would walk further across the screen on every save.
func editorReapplyMotifScreen(m *Motif, query string) {
	if m == nil {
		return
	}
	scope := editorRefreshMotifScope(m, query)
	if !scope.IsValid() {
		return
	}
	if !editorHasPopulatedSnapshot(scope) {
		// The screen has not been built (the motif is still loading, or this is a
		// bare test fixture): there is nothing to recompute, and the position
		// pass would dereference the nil snapshots.
		return
	}
	editorRefillSnapshots(m, scope, scope)
	m.applyPostParsePosAdjustments()
	// The script also reads plain values straight out of its own copy of the
	// motif table, which nothing above touches.
	editorSyncMotifLuaTable(m, query)
}

// editorSelectGridQuery reports whether an applied key needs the select
// screen's cell grid rebuilt: its size (rows / columns), the offset and
// spacing baked into every cell of start.t_grid when the script builds it, or
// the values baked into the cached draw list built from that grid (pos,
// showemptyboxes). Drawing reads the rest of a cell override (scale,
// facing, ...) straight from the motif, so only these need the rebuild —
// which also flags the draw list itself for rebuild.
func editorSelectGridQuery(query string) bool {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(query)), ".")
	if len(parts) < 2 || parts[0] != "select_info" {
		return false
	}
	switch parts[1] {
	case "rows", "columns", "pos", "showemptyboxes":
		return true
	case "cell":
		if len(parts) < 3 {
			return false
		}
		// cell.size / cell.spacing, and the per-cell overrides that are baked
		// into start.t_grid (cell.<c>-<r>.offset / .spacing / .skip).
		if len(parts) == 3 {
			return parts[2] == "size" || parts[2] == "spacing"
		}
		switch parts[len(parts)-1] {
		case "offset", "spacing", "skip":
			return true
		}
	}
	return false
}

// editorMotifMenuBuildKey reports whether a key is read by the menu builders
// (main.f_start / menu.f_start / options.f_start) into the menu tables they
// build once at load. The editor's reload re-runs those builders
// (main.f_rebuildMenus), so a save to one of these asks for a reload rather
// than a live apply that the menu tables would not reflect: the itemname labels
// and the section title drawn at the top of a menu are copied, while
// itemname_order is generated in Go.
func editorMotifMenuBuildKey(query string) bool {
	// Switches which menu (title vs attract) the script builds.
	if query == "attract_mode.enabled" {
		return true
	}
	// menu.itemname.<name> labels, at any depth (keymenu.itemname.<name>).
	parts := strings.Split(query, ".")
	for i, p := range parts {
		if p == "itemname" && i+1 < len(parts) {
			return true
		}
	}
	// <section>.menu.title.uppercase, applied when the labels are built.
	if strings.HasSuffix(query, ".menu.title.uppercase") {
		return true
	}
	// The section title is copied into the menu title (main.menu.title /
	// options.menu.title) when the menu is built.
	switch query {
	case "title_info.title.text", "option_info.title.text", "attract_mode.title.text":
		return true
	}
	return false
}

// editorReloadTails forces a reload for position and text-geometry tails
// under the listed query prefixes (trailing dot included). Those values only
// reach the screen through a reload: a live apply would leave the block
// drawing the boot copy. Scoped per section on purpose: the same tails apply
// live anywhere else, through the snapshot refill and the position pass
// (every [Title Info] key, for example), so a blanket *.offset / *.scale /
// *.pos / *.font rule would demote hundreds of working keys. A new scope is
// one table row.
var editorReloadTails = []struct {
	prefix string
	tails  []string
	reason string
}{
	{"select_info.stage.", []string{"pos", "offset", "scale"},
		"the select stage block is laid out when the motif loads"},
}

// editorMotifReloadTailKey reports whether a key ends in one of the
// reload-forcing tails of its section (editorReloadTails): stage.pos, the
// stage offset / scale, the offset / scale of the active / active2 / done
// stage texts, and whatever later rows add.
func editorMotifReloadTailKey(query string) (bool, string) {
	for _, rt := range editorReloadTails {
		if !strings.HasPrefix(query, rt.prefix) {
			continue
		}
		tail := query[strings.LastIndex(query, ".")+1:]
		for _, t := range rt.tails {
			if tail == t {
				return true, rt.reason
			}
		}
	}
	return false, ""
}

// editorCallLuaMethod runs a zero argument method of a global Lua table on the
// engine thread. A missing table or method (an older script, or a unit test
// with no Lua state) is a no-op, so callers do not have to guard.
func editorCallLuaMethod(table, method, what string) {
	l := sys.luaLState
	if l == nil {
		return
	}
	tbl, ok := l.GetGlobal(table).(*lua.LTable)
	if !ok {
		return
	}
	fn, ok := tbl.RawGetString(method).(*lua.LFunction)
	if !ok {
		return
	}
	top := l.GetTop()
	defer l.SetTop(top)
	if err := l.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}); err != nil {
		LogMessage("[Editor] unable to rebuild %v: %v", what, err)
	}
}

// editorRebuildSelectGrid rebuilds the select screen's cell grid after rows,
// columns, the cell geometry, pos or showemptyboxes changed
// (start.f_updateGrid). The grid and the draw list built from it are assembled
// once when the script loads, so this is what makes the edit reach the screen.
func editorRebuildSelectGrid() {
	editorCallLuaMethod("start", "f_updateGrid", "the select grid")
}

// editorSelectTitleQuery reports whether an applied key touched the [Select
// Info] title the script draws at the top of the select screen: its offset /
// font / layerno, or one of the mode keyed title.text.<mode> entries.
func editorSelectTitleQuery(query string) bool {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(query)), ".")
	return len(parts) >= 2 && parts[0] == "select_info" && parts[1] == "title"
}

// editorRebuildSelectTitle re-applies the current mode's text to the select
// screen's title sprite (main.f_refreshSelectTitle). The sprite is refilled in
// place by the refresh, but its text is copied from the mode keyed map when a
// mode is picked, so an edited title.text.<mode> would not show until then.
func editorRebuildSelectTitle() {
	editorCallLuaMethod("main", "f_refreshSelectTitle", "the select title")
}

// editorRebuildMotifMenus rebuilds the menus the motif declares after a reload
// swapped the motif table (main.f_rebuildMenus): main.menu, the pause menus,
// the options menu, and the attract vs title group choice.
func editorRebuildMotifMenus() {
	editorCallLuaMethod("main", "f_rebuildMenus", "the motif menus")
}

// editorRebuildAfterReload re-runs every script builder whose structure is
// assembled once at load and would otherwise keep drawing the boot copy after a
// reload swapped the motif table: the menus (main.f_rebuildMenus), the select
// cell grid (start.f_updateGrid) and the select title text
// (main.f_refreshSelectTitle). The title rebuild is what makes the reload not
// blank the select title: the new motif table carries a fresh, empty title
// TextSprite, and its text is only copied in from the mode keyed map when a
// mode is picked (main.f_setSelectTitle).
func editorRebuildAfterReload() {
	editorRebuildMotifMenus()
	editorRebuildSelectGrid()
	editorRebuildSelectTitle()
}

// editorMotifApplyMode is what a motif key save can achieve.
type editorMotifApplyMode int

const (
	// editorApplyReload: the engine reads the value when the motif is loaded.
	editorApplyReload editorMotifApplyMode = iota
	// editorApplyLive: assigning the field is enough; drawing reads it directly.
	editorApplyLive
	// editorApplyRefresh: assigning the field is not enough, but the screen's
	// snapshots can be rebuilt in place, so the next draw shows the new value.
	editorApplyRefresh
	// editorApplyRestart: the value is consumed once while the script boots into
	// a structure nothing rebuilds (a required module, the select.def roster),
	// so not even a motif reload can apply it. The file is written and the
	// in-memory copy kept in line, but the engine only picks it up on restart.
	editorApplyRestart
)

// editorMotifApplyClassify decides what a save can achieve for a key, with the
// reason shown to the user when it is not applied.
func editorMotifApplyClassify(section, key string) (editorMotifApplyMode, string) {
	sec := strings.ToLower(strings.TrimSpace(section))
	k := strings.ToLower(strings.TrimSpace(key))
	// A "<name>bgdef" section is read by readBackGround when the screen loads,
	// not through the Motif struct.
	if strings.HasSuffix(sec, "bgdef") {
		return editorApplyReload, "background definitions are parsed when the screen loads"
	}
	// parseMusicSection builds the whole music table at load time.
	if sec == "music" || strings.HasSuffix(sec, "music") {
		return editorApplyReload, "the music table is built when the motif loads"
	}
	// localcoord is the root of PopulateDataPointers: every derived pointer in
	// the tree (Anim, TextSprite, PalFX, Rect, Fade) is built from it.
	if k == "localcoord" {
		return editorApplyReload, "it rebuilds every derived pointer in the tree"
	}
	if sec == "files" {
		// Values the script reads once while it boots, into a structure nothing
		// in the reload path rebuilds: the module require list (t_modules) and the
		// select.def roster (main.t_selGrid / main.t_selChars).
		switch k {
		case "module", "select":
			return editorApplyRestart, "the script reads it once at load, so it needs a restart"
		}
		// These name assets that loadFiles() reads and uploads when the motif
		// itself is (re)loaded, which the reload re-runs.
		switch k {
		case "spr", "snd", "fight", "glyphs", "model",
			"logo.storyboard", "intro.storyboard":
			return editorApplyReload, "it names a file the engine loads at startup"
		}
		if editorFontKeyRe.MatchString(k) {
			return editorApplyReload, "it needs the font to be loaded again"
		}
		return editorApplyLive, ""
	}
	query := editorMotifQuery(sec, k)
	// The menus the motif declares are built once, at script load, from the
	// itemname labels and the section title drawn at their top. The reload
	// re-runs those builders (main.f_rebuildMenus), so these keys ask for a
	// reload: assigning the field would leave the menu tables drawing the boot
	// copy.
	if editorMotifMenuBuildKey(query) {
		return editorApplyReload, "the motif menus are built once, and a reload rebuilds them"
	}
	// Position and text-geometry tails that only reach the screen through a
	// reload (editorReloadTails): a live apply would leave the block drawing
	// the boot copy.
	if ok, reason := editorMotifReloadTailKey(query); ok {
		return editorApplyReload, reason
	}
	// A value a load time pointer snapshots (TextProperties, AnimationProperties,
	// ...) is drawn from that snapshot, not from the field, so the snapshot has
	// to be refilled for the edit to show up.
	if ptrs := editorMotifKeyDerivedPtrs(query); len(ptrs) > 0 {
		if !editorHasRefillablePtr(ptrs) {
			// The only snapshot is an *Anim, which carries element state that
			// only a rebuild can reset, so it cannot be refilled in place.
			return editorApplyReload, "it needs the animation to be built again"
		}
		if !editorRefreshMotifScope(&sys.motif, query).IsValid() {
			return editorApplyReload, "it is under a user named section, which needs a reload"
		}
		return editorApplyRefresh, ""
	}
	return editorApplyLive, ""
}

// editorHasRefillablePtr reports whether one of the snapshots can be refilled in
// place. A *Fade picks its *Anim up along the way (setFadeInto reads AnimData),
// so a struct declaring both is still refilled as a whole.
func editorHasRefillablePtr(ptrs []reflect.Type) bool {
	for _, p := range ptrs {
		switch p {
		case editorTextSpriteType, editorPalFxType, editorRectType, editorFadeType:
			return true
		}
	}
	return false
}

// editorMotifKeyNeedsReload reports whether a motif key needs a reload rather
// than being applied. The second result is the reason shown to the user.
func editorMotifKeyNeedsReload(section, key string) (bool, string) {
	mode, reason := editorMotifApplyClassify(section, key)
	return mode == editorApplyReload, reason
}

// editorIsMotifPath reports whether p is the motif the engine is running from.
func editorIsMotifPath(p string) bool {
	m := editorMotifPath()
	if m == "" {
		return false
	}
	a, err1 := editorSandboxPath(p)
	b, err2 := editorSandboxPath(m)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// editorDeleteMotifKey drops a key from the in-memory copy, matching the file
// and section name the way the engine parsers do (case insensitively).
func editorDeleteMotifKey(m *Motif, section, key string) {
	for _, sec := range m.IniFile.Sections() {
		if !strings.EqualFold(sec.Name(), strings.TrimSpace(section)) {
			continue
		}
		for _, k := range sec.Keys() {
			if strings.EqualFold(k.Name(), strings.TrimSpace(key)) {
				sec.DeleteKey(k.Name())
				return
			}
		}
	}
}

// editorSetMotifKey sets one key in the in-memory copy by its literal name,
// creating the section or key when needed. It is the fallback for keys the
// reflection walk cannot address (a background definition key, say), where the
// editor still knows the section and key exactly as they appear in the file.
func editorSetMotifKey(m *Motif, section, key, value string) {
	name := strings.TrimSpace(section)
	var sec *ini.Section
	for _, s := range m.IniFile.Sections() {
		if strings.EqualFold(s.Name(), name) {
			sec = s
			break
		}
	}
	if sec == nil {
		var err error
		if sec, err = m.IniFile.NewSection(name); err != nil {
			LogMessage("[Editor] unable to add the section %v to the in-memory motif: %v", name, err)
			return
		}
	}
	for _, k := range sec.Keys() {
		if strings.EqualFold(k.Name(), strings.TrimSpace(key)) {
			k.SetValue(value)
			return
		}
	}
	if _, err := sec.NewKey(strings.TrimSpace(key), value); err != nil {
		LogMessage("[Editor] unable to add the key %v to the in-memory motif: %v", key, err)
	}
}

// editorMotifQuery builds the "<section>.<key>" path the loader uses. A file
// spells a section for the reader ("[Title Info]") while the struct field is
// tagged with the engine's own name ("title_info"), and the loader bridges the
// two by lowercasing and turning spaces into underscores (motif.go, "Normalize
// spaces"). resolveSectionForWrite reverses that when writing the file back.
func editorMotifQuery(section, key string) string {
	norm := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "_"))
	}
	return norm(section) + "." + norm(key)
}

// editorApplyMotifValue performs the in-memory half of a motif save. It must
// run on the engine thread (see editorRunOnMainThread).
func editorApplyMotifValue(m *Motif, section, key, value string, remove bool, mode editorMotifApplyMode) error {
	if m == nil {
		return fmt.Errorf("the motif is not loaded")
	}
	if m.IniFile == nil {
		return fmt.Errorf("the motif has no in-memory copy")
	}
	if remove {
		editorDeleteMotifKey(m, section, key)
		return nil
	}
	query := editorMotifQuery(section, key)
	if mode == editorApplyReload || mode == editorApplyRestart {
		// Record the value but leave the live struct alone: the key only means
		// something once the load time passes (or a restart) run again. A key
		// the reflection walk does not know (a background definition key) falls
		// back to a literal write, so the in-memory copy never drifts either
		// way.
		if err := updateINIFile(m, m.IniFile, query, value); err != nil {
			editorSetMotifKey(m, section, key, value)
		}
		return nil
	}
	if err := SetValueUpdate(m, m.IniFile, query, value); err != nil {
		return err
	}
	switch mode {
	case editorApplyRefresh:
		if m.Sff == nil {
			// setTextSpriteInto builds a TextSprite out of a sprite reference, so
			// it needs the sprite file the motif was built with.
			return fmt.Errorf("the motif has no sprite file loaded, so its snapshots cannot be rebuilt")
		}
	}
	// A live field may still be consumed by a load time pass: menu.pos is not
	// drawn, it is added to the position of the already built TextSprites by
	// applyPostParsePosAdjustments. Re-applying the screen covers both cases, and
	// it is harmless when nothing was snapshotted from the key.
	editorReapplyMotifScreen(m, query)
	// rows / columns, the cell geometry, pos and showemptyboxes are baked into
	// the select screen's Lua grid and the draw list built from it when the
	// script loads, so the grid has to be rebuilt for the edit to show on the
	// next frame.
	if editorSelectGridQuery(query) {
		editorRebuildSelectGrid()
	}
	// The select title's text is a mode keyed map the script copies into the
	// title sprite when a mode is picked, so the refill alone leaves an edited
	// title.text.<mode> unread: ask the script to re-apply the current mode's
	// text (main.f_refreshSelectTitle).
	if editorSelectTitleQuery(query) {
		editorRebuildSelectTitle()
	}
	return nil
}

// editorFontResolutionWarning reports when a saved font index cannot change
// what the engine draws: the refill resolves the font through the motif's
// loaded fonts, so an index with no [Files] entry (or a value that is not a
// font array at all) leaves the snapshot pointing at the old font while the
// file and the struct hold the new one. A bare "applied" would lie about that,
// so the save reports it instead. Negative indices (the -1 default) are not
// fonts and never warn.
func editorFontResolutionWarning(m *Motif, query string) string {
	parts := parseQueryPath(query)
	if len(parts) == 0 || !strings.EqualFold(parts[len(parts)-1].name, "font") {
		return ""
	}
	field, ok := editorFieldByQuery(m, query)
	if !ok {
		return ""
	}
	idx := -1
	switch field.Kind() {
	case reflect.Array:
		if field.Len() == 0 || field.Index(0).Kind() < reflect.Int || field.Index(0).Kind() > reflect.Int64 {
			return ""
		}
		idx = int(field.Index(0).Int())
	default:
		return ""
	}
	if idx < 0 || m == nil {
		return ""
	}
	// A nil map (a bare test fixture, say) means nothing was ever loaded:
	// every non-negative index is unresolvable. reserveUserFontSlots blocks
	// taken slots with nil placeholders, which are not usable fonts either.
	if f, ok := m.Fnt[idx]; !ok || f == nil {
		return fmt.Sprintf("font %d is not loaded, so the text keeps the old font", idx)
	}
	return ""
}

// editorApplyMotifSync brings the running motif back in line with an editor
// edit. The file on disk is always written first; this keeps the in-memory copy
// from drifting (so a later Motif.Save cannot write back the old value) and,
// when the key allows it, pushes the value into the live struct so the change
// shows without a restart.
//
// applied reports whether the running engine now draws the new value, and mode
// is how the save was classified so the caller can tell a reload-only save from
// one that needs a restart. warning reports when it applied but part of the
// value could not take effect (an unloaded font index), so the caller can say so
// instead of a bare "applied".
func editorApplyMotifSync(section, key, value string, remove bool) (applied bool, mode editorMotifApplyMode, reason, warning string, err error) {
	mode = editorApplyReload
	out, err := editorRunOnEngineThread(func() editorApplyOutcome {
		// Classification reads the live motif to see whether the edited key's
		// screen can be scoped to a single struct field, so it happens here on
		// the engine thread, never on the HTTP goroutine.
		m := editorApplyReload
		why := "removing a key restores its default, which needs a reload"
		if !remove {
			m, why = editorMotifApplyClassify(section, key)
		}
		mode = m
		if err := editorApplyMotifValue(&sys.motif, section, key, value, remove, m); err != nil {
			return editorApplyOutcome{reason: why, err: err}
		}
		outcome := editorApplyOutcome{applied: m == editorApplyLive || m == editorApplyRefresh, reason: why}
		if outcome.applied && !remove {
			// The struct and the snapshot are updated, but an unloaded font
			// index leaves the typeface behind: report it alongside applied
			// rather than as a reload reason, since reloading (or restarting)
			// would not load the font either.
			outcome.warning = editorFontResolutionWarning(&sys.motif, editorMotifQuery(section, key))
		}
		return outcome
	})
	if err != nil {
		return false, mode, "", "", err
	}
	return out.applied, mode, out.reason, out.warning, out.err
}

// editorSaveRequestJSON is the payload of /api/save.
type editorSaveRequestJSON struct {
	Path    string `json:"path"`
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Remove  bool   `json:"remove"`
}

// editorHandleSave writes a single key back into a .def / .ini file, keeping the
// rest of the file (ordering, comments, indentation) untouched.
func editorHandleSave(w http.ResponseWriter, r *http.Request) {
	body, ok := editorReadJSONRequest(w, r, true)
	if !ok {
		return
	}
	req := editorSaveRequestJSON{}
	if err := json.Unmarshal(body, &req); err != nil {
		editorWriteError(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}
	if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Section) == "" || strings.TrimSpace(req.Key) == "" {
		editorWriteError(w, http.StatusBadRequest, "path, section and key are required")
		return
	}
	// One save at a time: the file edit is a read-modify-write, and the live
	// apply has to reach the engine thread in the same order as the writes.
	editorSaveMu.Lock()
	defer editorSaveMu.Unlock()
	abs, err := editorSandboxPath(req.Path)
	if err != nil {
		editorWriteError(w, http.StatusBadRequest, "%v", err)
		return
	}
	if editorExistingFile(abs) == "" {
		editorWriteError(w, http.StatusNotFound, "file not found: %v", req.Path)
		return
	}
	if req.Remove {
		err = editorEditINIFile(abs, req.Section, req.Key, nil)
	} else {
		value := req.Value
		err = editorEditINIFile(abs, req.Section, req.Key, &value)
	}
	if err != nil {
		editorWriteError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	action := "saved"
	if req.Remove {
		action = "removed"
	}
	// The running engine mirrors the motif only, so push the edit into it when
	// this is the configured motif. Any other file is inert to the engine.
	applied, needsReload, needsRestart, reason, warning, applyErr := false, false, false, "", "", error(nil)
	if editorIsMotifPath(req.Path) {
		var mode editorMotifApplyMode
		applied, mode, reason, warning, applyErr = editorApplyMotifSync(req.Section, req.Key, req.Value, req.Remove)
		needsReload = applyErr == nil && !applied && mode == editorApplyReload
		needsRestart = applyErr == nil && !applied && mode == editorApplyRestart
	}
	if needsReload {
		// The key only takes effect once the motif is reloaded, so run that
		// reload now instead of leaving it as a manual step. When the reload
		// is refused (a match is running, ...) the save keeps its needsReload
		// answer and the user reloads later, as before. editorHandleSave holds
		// editorSaveMu here, so the no-lock core is the one to call.
		if _, err := editorReloadMotifLocked(); err == nil {
			applied, needsReload = true, false
			reason = ""
		} else {
			LogMessage("[Editor] automatic reload after saving [%v] %v refused: %v",
				req.Section, req.Key, err)
		}
	}
	if applyErr != nil {
		// The file is already written; only the live update failed. This is
		// reported apart from needsReload on purpose: a restart will not help a
		// key the engine has no field for, so do not advise one.
		LogMessage("[Editor] %v [%v] %v in %v, but the running engine was not updated: %v",
			action, req.Section, req.Key, req.Path, applyErr)
		reason = applyErr.Error()
	}
	LogMessage("[Editor] %v [%v] %v in %v (applied=%v needsReload=%v warning=%v err=%v)",
		action, req.Section, req.Key, req.Path, applied, needsReload, warning, applyErr)
	res := map[string]any{
		"ok":           true,
		"path":         req.Path,
		"section":      req.Section,
		"key":          req.Key,
		"value":        req.Value,
		"message":      action,
		"applied":      applied,
		"needsReload":  needsReload,
		"needsRestart": needsRestart,
	}
	if reason != "" {
		res["applyReason"] = reason
	}
	if warning != "" {
		res["applyWarning"] = warning
	}
	if applyErr != nil {
		res["applyError"] = applyErr.Error()
	}
	editorWriteJSON(w, res)
}

// editorReloadTimeout bounds how long a motif reload waits for the engine to
// pick the change up. A reload re-parses the motif and rebuilds its assets
// (SFF, backgrounds, fonts), so it gets more slack than a single key apply.
const editorReloadTimeout = 30 * time.Second

// editorMotifReloadBlocked reports why the motif cannot be reloaded right now,
// or "" when a reload is safe to attempt. The motif is drawn every frame and
// its snapshots are handed to the Lua script as handles, so rebuilding it in
// the middle of a match (or while assets are loading, or while netplay / a
// replay owns the game state) would tear down objects the frame is using.
func editorMotifReloadBlocked() string {
	if sys.middleOfMatch() || sys.gameRunning {
		return "a match is running"
	}
	if sys.netplay() {
		return "netplay or a replay is active"
	}
	if sys.loader.state == LS_Loading {
		return "assets are loading"
	}
	return ""
}

// editorReloadMotifSync reloads the configured motif from disk and hands it to
// the running script, mirroring boot (`motif = loadMotif()` in
// external/script/main.lua): the Lua loadMotif() global re-parses the file,
// swaps sys.motif, rebuilds the cached table (with the menu itemname overlays)
// and returns it, and the returned table replaces the script's `motif` global,
// which every menu screen reads every frame.
//
// Going through the Lua global instead of calling Go loadMotif directly is the
// whole point: replacing sys.motif alone leaves the script drawing the table
// it already holds, so no visual would change. A failed parse raises inside
// the protected call and leaves the running motif untouched.
//
// It must run on the engine thread (see editorRunOnMainThread): the call runs
// Lua code, and the game loop reads the motif every frame. The call is
// protected, matching the nested statusLFunc calls in system.go.
//
// editorReloadMotifSync takes editorSaveMu so a reload always runs after the
// write it follows. Code that already holds the lock (editorHandleSave) must
// call editorReloadMotifLocked instead; taking the lock again there would
// deadlock, since Go's sync.Mutex is not re-entrant.
func editorReloadMotifSync() (path string, err error) {
	editorSaveMu.Lock()
	defer editorSaveMu.Unlock()
	return editorReloadMotifLocked()
}

// editorReloadMotifLocked is the reload core. Callers must hold editorSaveMu.
func editorReloadMotifLocked() (path string, err error) {
	out, err := editorRunOnEngineThreadTimeout(editorReloadTimeout, func() editorApplyOutcome {
		if reason := editorMotifReloadBlocked(); reason != "" {
			return editorApplyOutcome{reason: reason}
		}
		path := editorMotifPath()
		if path == "" {
			return editorApplyOutcome{reason: "no motif configured"}
		}
		if sys.luaLState == nil {
			return editorApplyOutcome{reason: "the engine Lua state is not running"}
		}
		l := sys.luaLState
		fn := l.GetGlobal("loadMotif")
		if fn == lua.LNil {
			return editorApplyOutcome{err: fmt.Errorf("the engine has no loadMotif global")}
		}
		top := l.GetTop()
		defer l.SetTop(top)
		// No argument: like boot, the configured motif is resolved inside.
		if err := l.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
			return editorApplyOutcome{err: err}
		}
		tbl, ok := l.Get(-1).(*lua.LTable)
		if !ok {
			return editorApplyOutcome{err: fmt.Errorf("loadMotif did not return a motif table")}
		}
		l.SetGlobal("motif", tbl)
		// The menus the motif declares and the select screen's cell grid are
		// built once at script load, so a reload has to rebuild them too for an
		// edited itemname / attract-mode / rows / columns to show without
		// restarting the game.
		editorRebuildAfterReload()
		return editorApplyOutcome{applied: true}
	})
	if err != nil {
		return "", err
	}
	if out.err != nil {
		return "", out.err
	}
	if !out.applied {
		return "", fmt.Errorf("%s", out.reason)
	}
	return editorMotifPath(), nil
}

// ---------------------------------------------------------------------------
// Pinned keys ([Editor Pins] in the engine config)
// ---------------------------------------------------------------------------

// editorPinsSection is the config.ini section holding the editor's pinned
// keys. One line per pin, "path|section|key = 1": the triple itself is the
// INI key, so pinning appends a line and unpinning deletes it, with no
// numbering to keep in sync. The engine's config loader skips this section
// (editorOwnsConfigSection) instead of warning for every key.
const editorPinsSection = "Editor Pins"

// editorOwnsConfigSection reports whether a config.ini section belongs to
// the editor rather than the engine. The loader skips those instead of
// warning that the Config struct has no such field.
func editorOwnsConfigSection(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), editorPinsSection)
}

// editorPinsMu serializes pin toggles. A toggle reads the config, edits it
// line by line and writes it back, so two concurrent toggles could lose one.
var editorPinsMu sync.Mutex

// editorPinID joins a key's address into the single INI key stored for it.
func editorPinID(path, section, key string) string {
	return strings.TrimSpace(path) + "|" + strings.TrimSpace(section) + "|" + strings.TrimSpace(key)
}

// editorNormPinID normalizes a stored pin for comparison. Sections and keys
// match the way the engine parsers do (case insensitively); paths the same
// way, so Windows casing never duplicates a pin.
func editorNormPinID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// editorParsePins reads the pinned triples out of raw config text. Only the
// [Editor Pins] section is looked at; everything else passes through
// untouched by the line edit below.
func editorParsePins(text string) [][3]string {
	out := [][3]string{}
	in := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			name := t
			if j := strings.Index(name, "]"); j >= 0 {
				name = name[1:j]
			}
			in = strings.EqualFold(strings.TrimSpace(name), editorPinsSection)
			continue
		}
		if !in || t == "" || strings.HasPrefix(t, ";") {
			continue
		}
		left := t
		if i := strings.Index(left, "="); i >= 0 {
			left = strings.TrimSpace(left[:i])
		}
		parts := strings.Split(left, "|")
		if len(parts) != 3 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
			if parts[i] == "" {
				break
			}
		}
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			continue
		}
		out = append(out, [3]string{parts[0], parts[1], parts[2]})
	}
	return out
}

// editorSetPinFile pins or unpins one key in the config at cfgPath with a
// line based edit, so the rest of the file (ordering, comments, spacing) is
// preserved byte for byte. It reports whether the file changed.
func editorSetPinFile(cfgPath, path, section, key string, pinned bool) (bool, error) {
	id := editorPinID(path, section, key)
	if strings.TrimSpace(path) == "" || strings.TrimSpace(section) == "" || strings.TrimSpace(key) == "" {
		return false, fmt.Errorf("path, section and key are required")
	}
	norm := editorNormPinID(id)
	if _, err := editorFileWithinCap(cfgPath, editorMaxReadFile); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
		if !pinned {
			return false, nil
		}
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
			return false, err
		}
		raw = nil
	}
	lines := strings.Split(string(raw), "\n")
	head, end := -1, len(lines)
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "[") {
			continue
		}
		name := t
		if j := strings.Index(name, "]"); j >= 0 {
			name = name[1:j]
		} else {
			continue
		}
		if head < 0 && strings.EqualFold(strings.TrimSpace(name), editorPinsSection) {
			head = i
			continue
		}
		if head >= 0 {
			end = i
			break
		}
	}
	found := -1
	if head >= 0 {
		for i := head + 1; i < end; i++ {
			left := strings.TrimSpace(lines[i])
			if left == "" || strings.HasPrefix(left, ";") {
				continue
			}
			if j := strings.Index(left, "="); j >= 0 {
				left = strings.TrimSpace(left[:j])
			}
			if editorNormPinID(left) == norm {
				found = i
				break
			}
		}
	}
	switch {
	case pinned && found >= 0:
		return false, nil
	case pinned && head < 0:
		text := string(raw)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "[" + editorPinsSection + "]\n" + id + " = 1\n"
		if err := editorWriteFileAtomic(cfgPath, []byte(text)); err != nil {
			return false, err
		}
		return true, nil
	case pinned:
		lines = append(lines[:end], append([]string{id + " = 1"}, lines[end:]...)...)
	case found < 0:
		return false, nil
	default:
		lines = append(lines[:found], lines[found+1:]...)
	}
	if err := editorWriteFileAtomic(cfgPath, []byte(strings.Join(lines, "\n"))); err != nil {
		return false, err
	}
	return true, nil
}

// editorPinRequestJSON is the payload of POST /api/pins.
type editorPinRequestJSON struct {
	Path    string `json:"path"`
	Section string `json:"section"`
	Key     string `json:"key"`
	Pinned  bool   `json:"pinned"`
}

// editorHandlePins serves the pinned keys backing the editor's pin column:
// GET lists them, POST pins or unpins one in [Editor Pins] of the engine
// config, so the marks survive a restart.
func editorHandlePins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		pins := [][3]string{}
		cfgPath := editorConfigPath()
		if _, err := editorFileWithinCap(cfgPath, editorMaxReadFile); err != nil && !os.IsNotExist(err) {
			editorWriteError(w, editorFileReadStatus(err), "unable to read pins: %v", err)
			return
		}
		if raw, err := os.ReadFile(cfgPath); err == nil {
			pins = editorParsePins(string(raw))
		} else if !os.IsNotExist(err) {
			editorWriteError(w, http.StatusInternalServerError, "unable to read pins: %v", err)
			return
		}
		out := make([]map[string]string, 0, len(pins))
		for _, p := range pins {
			out = append(out, map[string]string{"path": p[0], "section": p[1], "key": p[2]})
		}
		editorWriteJSON(w, map[string]any{"ok": true, "pins": out})
	case http.MethodPost:
		body, ok := editorReadJSONRequest(w, r, false)
		if !ok {
			return
		}
		req := editorPinRequestJSON{}
		if err := json.Unmarshal(body, &req); err != nil {
			editorWriteError(w, http.StatusBadRequest, "invalid JSON: %v", err)
			return
		}
		if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Section) == "" || strings.TrimSpace(req.Key) == "" {
			editorWriteError(w, http.StatusBadRequest, "path, section and key are required")
			return
		}
		if _, err := editorSandboxPath(req.Path); err != nil {
			editorWriteError(w, http.StatusBadRequest, "invalid path: %v", err)
			return
		}
		editorPinsMu.Lock()
		changed, err := editorSetPinFile(editorConfigPath(), req.Path, req.Section, req.Key, req.Pinned)
		editorPinsMu.Unlock()
		if err != nil {
			editorWriteError(w, http.StatusInternalServerError, "unable to save pin: %v", err)
			return
		}
		editorWriteJSON(w, map[string]any{"ok": true, "pinned": req.Pinned, "changed": changed})
	default:
		editorWriteError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

// editorHandleReload reloads the configured motif from disk into the running
// engine, so edits to reload-only keys (background definitions, [Music],
// [Files] asset paths, localcoord, ...) show up without restarting the game.
// Only safe outside a match; use the Lua reload() global for mid-match char /
// stage / fight screen refreshes instead.
func editorHandleReload(w http.ResponseWriter, r *http.Request) {
	if _, ok := editorReadJSONRequest(w, r, true); !ok {
		return
	}
	// editorReloadMotifSync takes editorSaveMu itself: a reload re-reads the
	// file a save just wrote, so it has to run after the write lands and its
	// live apply.
	path, err := editorReloadMotifSync()
	if err != nil {
		msg := err.Error()
		code := http.StatusInternalServerError
		if msg == "a match is running" || msg == "netplay or a replay is active" ||
			msg == "assets are loading" {
			code = http.StatusConflict
		}
		if msg == "no motif configured" {
			code = http.StatusNotFound
		}
		LogMessage("[Editor] motif reload refused (%v): %v", path, msg)
		editorWriteError(w, code, "%s", msg)
		return
	}
	LogMessage("[Editor] motif reloaded from %v", path)
	editorWriteJSON(w, map[string]any{
		"ok": true, "path": path, "message": "reloaded",
	})
}

// editorEditINIFile updates (or removes, when value is nil) a single key inside
// a section, editing the file line by line.
func editorEditINIFile(path, section, key string, value *string) error {
	if _, err := editorFileWithinCap(path, editorMaxReadFile); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	crlf := strings.Contains(string(raw), "\r\n")
	lines := strings.Split(NormalizeNewlines(string(raw)), "\n")

	target := strings.TrimSpace(section)
	startIdx, endIdx := -1, len(lines)
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "[") {
			continue
		}
		end := strings.Index(t, "]")
		if end < 0 {
			continue
		}
		name := strings.TrimSpace(t[1:end])
		if startIdx >= 0 {
			endIdx = i
			break
		}
		if strings.EqualFold(name, target) {
			startIdx = i
		}
	}

	if startIdx < 0 {
		if value == nil {
			return fmt.Errorf("section [%v] not found", section)
		}
		out := NormalizeNewlines(string(raw))
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += "[" + target + "]\n" + key + " = " + *value + "\n"
		return editorWriteTextFile(path, out, crlf)
	}

	keyIdx := -1
	for i := startIdx + 1; i < endIdx; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, ";") {
			continue
		}
		eq := strings.Index(t, "=")
		if eq < 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(t[:eq]), strings.TrimSpace(key)) {
			keyIdx = i
			break
		}
	}

	if value == nil {
		if keyIdx < 0 {
			return nil
		}
		lines = append(lines[:keyIdx], lines[keyIdx+1:]...)
		return editorWriteTextFile(path, strings.Join(lines, "\n"), crlf)
	}

	if keyIdx >= 0 {
		// Keep the original key spelling and any trailing inline comment,
		// including the run of whitespace in front of it, so re-saving a value
		// that did not change leaves the file byte for byte identical.
		line := lines[keyIdx]
		eq := strings.Index(line, "=")
		prefix := line[:eq+1]
		comment := ""
		if sc := strings.Index(line[eq+1:], ";"); sc >= 0 {
			// The comment runs from the whitespace in front of the semicolon
			// to the end of the line. Walk back over that whitespace only, so
			// the value in between is not swallowed.
			start := eq + 1 + sc
			for start > eq+1 && (line[start-1] == ' ' || line[start-1] == '\t') {
				start--
			}
			comment = line[start:]
		}
		lines[keyIdx] = prefix + " " + *value + comment
	} else {
		insertAt := endIdx
		for insertAt > startIdx+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		newLine := key + " = " + *value
		lines = append(lines[:insertAt], append([]string{newLine}, lines[insertAt:]...)...)
	}
	return editorWriteTextFile(path, strings.Join(lines, "\n"), crlf)
}

// editorWriteFileAtomic writes data to path through a temporary file in the
// same directory and an atomic rename, so a crash or a full disk mid-write
// leaves the previous contents intact instead of a truncated file. The
// original permission bits are kept.
func editorWriteFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("unable to create a temporary file next to %v: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("unable to write %v: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("unable to set the mode of %v: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("unable to close %v: %w", path, err)
	}
	// os.Rename replaces an existing target on every supported platform
	// (Windows included, through MoveFileEx with REPLACE_EXISTING), so the
	// swap is atomic from a reader's point of view.
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("unable to replace %v: %w", path, err)
	}
	return nil
}

// editorWriteTextFile writes text back, restoring the original line endings.
func editorWriteTextFile(path, text string, crlf bool) error {
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if err := editorWriteFileAtomic(path, []byte(text)); err != nil {
		return fmt.Errorf("unable to write %v: %w", path, err)
	}
	return nil
}

// editorHandleCharacters serves the character list built from select.def.
func editorHandleCharacters(w http.ResponseWriter, r *http.Request) {
	sel := editorSelectDefPath()
	if _, err := editorFileWithinCap(sel, editorMaxReadFile); err != nil {
		editorWriteError(w, editorFileReadStatus(err), "unable to read %v: %v", sel, err)
		return
	}
	text, err := LoadText(sel)
	if err != nil {
		editorWriteError(w, http.StatusNotFound, "unable to read %v: %v", sel, err)
		return
	}
	characters, _ := editorSelectDefSections(text)
	editorWriteJSON(w, map[string]any{
		"ok":         true,
		"selectDef":  sel,
		"characters": characters,
	})
}

// ---------------------------------------------------------------------------
// Editor UI (single page, no external assets)
// ---------------------------------------------------------------------------

// editorPageHTML is served at "/" and is intentionally dependency free. Each
// tab mirrors one engine view: Motif (Motif struct layout + motif INI values),
// Stage and Character (select.def entries and their def files).
const editorPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Ikemen GO Editor</title>
<style>
:root{--bg:#11141a;--panel:#181d26;--line:#2a3140;--fg:#e6e9ef;--dim:#8d97ab;--acc:#4da3ff;--ok:#39d353;--warn:#e3b341;--err:#ff6b6b}
*{box-sizing:border-box}
body{margin:0;font:13px/1.5 "Segoe UI",system-ui,sans-serif;background:var(--bg);color:var(--fg)}
header{display:flex;flex-wrap:wrap;gap:12px;align-items:center;padding:10px 16px;background:var(--panel);border-bottom:1px solid var(--line);position:sticky;top:0;z-index:5}
h1{font-size:15px;margin:0;font-weight:600}
nav button{background:transparent;border:1px solid var(--line);color:var(--dim);padding:5px 12px;border-radius:6px;cursor:pointer}
nav button.active{border-color:var(--acc);color:var(--fg);background:#1d2534}
#status{margin-left:auto;color:var(--dim);font-size:12px}
main{padding:16px}
.grid{display:grid;grid-template-columns:340px 1fr;gap:16px;align-items:start}
.panel{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:12px}
.small{font-size:11px;color:var(--dim)}
.keyfilter{display:flex;gap:8px;align-items:center;margin:0 0 8px}
.keyfilter input{flex:1;max-width:280px;background:#0e1117;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:4px 6px;font-size:12px}
.sffrow{display:flex;gap:6px;align-items:center;margin:4px 0}
.sffrow label{display:flex;gap:4px;align-items:center;color:var(--dim);font-size:11px}
.sffrow input[type=text],.sffrow input:not([type]){flex:1;min-width:0;background:#0e1117;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:4px 6px;font-size:12px}
.sffrow input#sff-group,.sffrow input#sff-number{width:64px;background:#0e1117;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:4px 6px;font-size:12px}
.sffimg{display:block;margin-top:6px;min-height:24px;min-width:24px;
	max-width:100%;max-height:50vh;width:auto;height:auto;
	image-rendering:pixelated;background:
	linear-gradient(45deg,#1a2030 25%,transparent 25%,transparent 75%,#1a2030 75%),
	linear-gradient(45deg,#1a2030 25%,#151b28 25%,#151b28 75%,#1a2030 75%);
	background-size:12px 12px;background-position:0 0,6px 6px}
table{width:100%;border-collapse:collapse}
th,td{text-align:left;padding:4px 6px;border-bottom:1px solid var(--line);vertical-align:top}
th{color:var(--dim);font-weight:600;font-size:11px;text-transform:uppercase;letter-spacing:.04em}
td.k{font-family:Consolas,monospace;color:#b7c4dc;white-space:nowrap}
td.v input{width:100%;min-width:180px;background:#0e1117;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:3px 6px;font-family:Consolas,monospace}
td.v select{width:100%;min-width:180px;background:#0e1117;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:3px 6px;font-family:Consolas,monospace}
td.a{white-space:nowrap}
td.pin{white-space:nowrap;width:1%}
button.mini{background:#22303f;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:3px 8px;cursor:pointer}
button.mini:hover{border-color:var(--acc)}
.tag{font-size:10px;padding:1px 5px;border-radius:4px;border:1px solid var(--line);color:var(--dim)}
.tag.ok{color:var(--ok);border-color:#1d4d29}
.tag.miss{color:var(--dim);border-color:var(--line)}
.tag.un{color:var(--err);border-color:#4d1d1d}
.hint{color:var(--dim);font-size:12px;margin:0 0 12px}
.sectree{max-height:70vh;overflow:auto;font-size:12px}
.sectree ul{list-style:none;margin:0;padding-left:13px}
.sectree summary{list-style:none;cursor:pointer;display:flex;gap:8px;align-items:center;padding:2px 6px;border-radius:5px}
.sectree summary::-webkit-details-marker{display:none}
.sectree summary::before{content:"\25B8";color:var(--dim);font-size:10px;display:inline-block;width:10px}
.sectree details[open]>summary::before{content:"\25BE"}
.sectree details.grp>summary{color:var(--acc);font-weight:600;margin-top:4px}
.sectree .row{cursor:pointer;display:flex;gap:8px;align-items:center;padding:2px 6px;border-radius:5px}
.sectree summary:hover,.sectree .row:hover{background:#222a38}
.sectree .sel{background:#243247;color:#fff}
.sectree .t{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.raw{margin:0;padding:8px 10px;background:#0e1117;border:1px solid var(--line);border-radius:6px;
	font-family:Consolas,monospace;font-size:12px;color:#c8d2e4;white-space:pre-wrap;word-break:break-word;
	max-height:60vh;overflow:auto}
select{background:#0e1117;border:1px solid var(--line);color:var(--fg);border-radius:4px;padding:4px 6px;max-width:100%}
#toast{position:fixed;bottom:16px;right:16px;background:#1d2534;border:1px solid var(--line);border-radius:8px;padding:8px 12px;opacity:0;transition:opacity .2s;z-index:9}
#toast.show{opacity:1}
.hidden{display:none}
</style>
</head>
<body>
<header>
  <h1>Ikemen GO Editor</h1>
  <nav>
    <button data-view="motif" class="active">Motif</button>
    <button data-view="stage">Stage</button>
    <button data-view="character">Character</button>
  </nav>
  <span id="status">loading...</span>
</header>
<main>
  <section id="view-motif">
    <div class="hint" id="motif-hint"></div>
    <div style="margin:0 0 12px"><button class="mini" id="motif-reload">Reload motif in engine</button>
    <span class="small">re-reads the motif file into the running engine (blocked during a match)</span></div>
    <div class="grid">
      <div class="panel">
        <div class="small">Sections</div>
        <div class="sectree" id="motif-sections"></div>
      </div>
      <div class="panel">
        <div class="small" id="motif-title"></div>
        <div id="motif-keys"></div>
      </div>
    </div>
  </section>
  <section id="view-stage" class="hidden">
    <div class="hint" id="stage-hint"></div>
    <div class="grid">
      <div class="panel">
        <div class="small">Stage</div>
        <select id="stage-list" style="margin:6px 0"></select>
        <div class="small">Sections</div>
        <div class="sectree" id="stage-sections"></div>
      </div>
      <div class="panel">
        <div class="small">Sprite preview</div>
        <div class="sffrow">
          <input id="stage-sff-file" placeholder="stages/stage0.sff" spellcheck="false">
        </div>
        <div class="sffrow">
          <label>group <input id="stage-sff-group" value="0" size="5"></label>
          <label>number <input id="stage-sff-number" value="0" size="5"></label>
          <button id="stage-sff-show" class="mini">Show</button>
        </div>
        <img id="stage-sff-img" class="sffimg" alt="sprite preview">
        <div class="small" id="stage-sff-hint"></div>
        <div class="small" id="stage-title"></div><div id="stage-keys"></div>
      </div>
    </div>
  </section>
  <section id="view-character" class="hidden">
    <div class="hint" id="character-hint"></div>
    <div class="grid">
      <div class="panel">
        <div class="small">Character</div>
        <select id="character-list" style="margin:6px 0"></select>
        <div class="small">Sections</div>
        <div class="sectree" id="character-sections"></div>
      </div>
      <div class="panel">
        <div class="small">Sprite preview</div>
        <div class="sffrow">
          <input id="character-sff-file" placeholder="chars/kfm/kfm.sff" spellcheck="false">
        </div>
        <div class="sffrow">
          <label>group <input id="character-sff-group" value="0" size="5"></label>
          <label>number <input id="character-sff-number" value="0" size="5"></label>
          <button id="character-sff-show" class="mini">Show</button>
        </div>
        <img id="character-sff-img" class="sffimg" alt="sprite preview">
        <div class="small" id="character-sff-hint"></div>
        <div class="small" id="character-title"></div><div id="character-keys"></div>
      </div>
    </div>
  </section>
</main>
<div id="toast"></div>
<script>
var state = { motif: null, sec: 0, page: { path: '', sections: [], index: 0 } };
var treeIndex = {};   // rendered tree id -> section node
var selName = null;   // section name kept across reloads
// The .def views (stage, character) share one tree renderer, so each keeps its
// own tree index and selected section: view id -> { index, sel }.
var fileViews = {};
function viewState(id) {
	if (!fileViews[id]) { fileViews[id] = { index: {}, sel: null }; }
	return fileViews[id];
}

function el(id) { return document.getElementById(id); }

function esc(s) {
	return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}
function escAttr(s) {
	return esc(s).replace(/"/g, '&quot;');
}
function toast(msg, bad) {
	var t = el('toast');
	t.textContent = msg;
	t.style.borderColor = bad ? '#ff6b6b' : '#39d353';
	t.classList.add('show');
	setTimeout(function () { t.classList.remove('show'); }, 2500);
}
function api(url) {
	return fetch(url).then(function (r) { return r.json(); });
}
function post(payload) {
	return fetch('/api/save', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', 'X-Editor-Request': '1' },
		body: JSON.stringify(payload)
	}).then(function (r) { return r.json(); });
}
function setView(view) {
	var views = ['motif', 'stage', 'character'];
	if (views.indexOf(view) < 0) { view = 'motif'; }
	views.forEach(function (v) {
		el('view-' + v).classList.toggle('hidden', v !== view);
	});
	Array.prototype.forEach.call(document.querySelectorAll('nav button'), function (b) {
		b.classList.toggle('active', b.getAttribute('data-view') === view);
	});
	if (window.history.replaceState) {
		window.history.replaceState(null, '', '/?view=' + view);
	}
	if (view === 'stage' && !el('stage-list').childNodes.length) { loadStages(); }
	if (view === 'character' && !el('character-list').childNodes.length) { loadCharacters(); }
}

/* ---- shared key table ---- */
function badge(k) {
	// Three states, three icons: ● green = the file sets it and the engine
	// reads it, ○ grey = the engine default applies, ⚠ red = the engine
	// ignores the key. The full state name rides along as a tooltip.
	if (!k.defined) { return '<span class="tag miss" title="missing">○</span>'; }
	if (k.unknown) { return '<span class="tag un" title="unknown">⚠</span>'; }
	return '<span class="tag ok" title="defined">●</span>';
}
// keyFilter keeps the text typed into each key table's filter box, so a
// re-render (after a save or a reload) keeps what the user filtered on.
// containers are addressed by their element id (motif-keys, stage-keys, ...).
var keyFilter = {};
// pins holds the pinned keys by "path\nsection\nkey", so a marked row floats
// to the top of its table. It is filled from /api/pins at startup and kept
// across re-renders; the config file on disk is the source of truth.
// lastRender keeps each key table's last arguments, so a pin toggle can
// re-render the same section without losing the filter text.
var pins = {};
var lastRender = {};
function pinId(path, section, key) {
	return path + '\n' + section + '\n' + key;
}
// loadPins fetches the pins stored in [Editor Pins] of the engine config. It
// never rejects: without pins the tables simply render unpinned.
function loadPins() {
	return api('/api/pins').then(function (p) {
		pins = {};
		((p && p.pins) || []).forEach(function (pin) {
			pins[pinId(pin.path, pin.section, pin.key)] = true;
		});
	}).catch(function () { pins = {}; });
}
// keyFilterMatch reports whether a lowercased KEY cell matches the lowercased
// filter text. A "*" is a wildcard for any (possibly empty) run, so
// "font*offset" finds every font offset key: the pieces around each "*"
// must appear in order. Without "*" it is a plain substring, as before.
function keyFilterMatch(text, q) {
	var parts = q.split('*');
	var pos = 0;
	for (var i = 0; i < parts.length; i++) {
		if (!parts[i]) { continue; }
		pos = text.indexOf(parts[i], pos);
		if (pos < 0) { return false; }
		pos += parts[i].length;
	}
	return true;
}
// applyKeyFilter hides the rows whose KEY column does not match the text in
// the container's filter box and reports how many rows are shown. Matching is
// a case-insensitive substring, so "font" finds every font key of a section;
// "*" wildcards any run, so "font*offset" narrows it to the font offsets.
function applyKeyFilter(container) {
	var q = String(keyFilter[container.id] || '').trim().toLowerCase();
	var rows = container.querySelectorAll('tbody tr');
	var shown = 0;
	Array.prototype.forEach.call(rows, function (tr) {
		var cell = tr.querySelector('td.k');
		var hit = !q || (cell && keyFilterMatch(cell.textContent.toLowerCase(), q));
		tr.classList.toggle('hidden', !hit);
		if (hit) { shown++; }
	});
	var note = container.querySelector('.keyfilter-count');
	if (note) {
		note.textContent = q ? shown + ' / ' + rows.length + ' keys' : '';
	}
}
// renderKeys renders the key table. plain drops the State column: the motif
// view has a schema behind every key so the state says something, while a
// .def opened from the Stage view has none. Type and default never had a
// column of their own: they ride along as a tooltip on the key name and on
// the value control. The first column pins the row: pinned keys float to the
// top and the mark is stored in [Editor Pins] of the engine config. A filter
// box above the table narrows the rows by the KEY column's text.
function renderKeys(container, path, section, keys, reload, plain) {
	if (!keys.length) {
		container.innerHTML = '<div class="small">no keys</div>';
		return;
	}
	// Pinned rows float to the top. Stable by construction: each group keeps
	// the order the service sent. The indexes below address this order, and
	// the same order is cached for the pin toggle's re-render.
	var pinnedRows = [], restRows = [];
	keys.forEach(function (k) {
		(pins[pinId(path, section, k.key)] ? pinnedRows : restRows).push(k);
	});
	keys = pinnedRows.concat(restRows);
	lastRender[container.id] = { path: path, section: section, keys: keys, reload: reload, plain: plain };
	var h = plain
		? ['<table><thead><tr><th>Pin</th><th>Key</th><th>Value</th><th></th></tr></thead><tbody>']
		: ['<table><thead><tr><th>Pin</th><th>Key</th><th>State</th><th>Value</th><th></th></tr></thead><tbody>'];
	keys.forEach(function (k, i) {
		// Tooltip for the key cell and the value control: "type — default X",
		// or just the type when the engine has no fixed default. A blank
		// default means the parser leaves the zero value alone; the sentinels
		// below are the engine's own "unset" markers spelled out.
		var meta = k.type || '';
		var def = k.default || '';
		if (def === '' && (k.key === 'zoomdelta' || k.key === 'zoomscaledelta' || k.key === 'xbottomzoomdelta')) {
			def = '(unset)';
		} else if (def === '' && k.key === 'roundpos') {
			def = '(stage default)';
		}
		var tip = meta;
		if (def) { tip += (tip ? ' — default ' : 'default ') + def; }
		h.push('<tr>');
		var isPin = !!pins[pinId(path, section, k.key)];
		h.push('<td class="pin"><button class="mini" data-pin="' + i + '" title="'
			+ (isPin ? 'pinned — click to unpin' : 'pin this key') + '">'
			+ (isPin ? '◈' : '◇') + '</button></td>');
		h.push('<td class="k"' + (tip ? ' title="' + escAttr(tip) + '"' : '') + '>' + esc(k.key) + '</td>');
		if (!plain) {
			h.push('<td>' + badge(k) + '</td>');
		}
		// Enumerated keys (type, trans, projection, ...) become a combo box of
		// the values the engine accepts; everything else stays a text field.
		var value = k.value == null ? '' : k.value;
		var field;
		if (k.choices && k.choices.length) {
			// Always show the value that is in effect: the stored one, or the
			// engine default for a key the file does not define yet. The engine
			// compares these case insensitively, so casing never hides a match.
			var target = value;
			if (target === '' && k.defined === false && k.default) {
				target = k.default;
			}
			// Only the values the engine accepts, no blank entry.
			var opts = [];
			var known = false;
			k.choices.forEach(function (c) {
				var sel = '';
				if (String(c).toLowerCase() === String(target).toLowerCase()) {
					sel = ' selected';
					known = true;
				}
				opts.push('<option value="' + escAttr(c) + '"' + sel + '>' + esc(c) + '</option>');
			});
			if (value !== '' && !known) {
				// A value the engine does not know: keep it, flagged as custom.
				opts.push('<option value="' + escAttr(value) + '" selected>' + esc(value) + ' (custom)</option>');
			}
			field = '<select id="kv-' + i + '" data-value="' + escAttr(target) + '"'
				+ (tip ? ' title="' + escAttr(tip) + '"' : '') + '>'
				+ opts.join('') + '</select>';
		} else {
			field = '<input id="kv-' + i + '" value="' + escAttr(value) + '" placeholder="' + escAttr(k.default || '') + '"'
				+ (tip ? ' title="' + escAttr(tip) + '"' : '') + '">';
		}
		h.push('<td class="v">' + field + '</td>');
		h.push('<td class="a"><button class="mini" data-save="' + i + '">Save</button>');
		if (k.defined) { h.push(' <button class="mini" data-del="' + i + '">Del</button>'); }
		h.push('</td></tr>');
	});
	h.push('</tbody></table>');
	container.innerHTML = '<div class="keyfilter">'
		+ '<input type="text" class="keyfilter-input" placeholder="Filter keys (* wildcards)\u2026"'
		+ ' value="' + escAttr(keyFilter[container.id] || '') + '" spellcheck="false">'
		+ '<span class="small keyfilter-count"></span></div>'
		+ h.join('');
	// Live filter: as the box changes, only the matching rows stay visible. It
	// hides rows rather than re-rendering, so the Save / Del button indexes and
	// the input's focus are untouched. The filter text survives a re-render.
	var input = container.querySelector('.keyfilter-input');
	if (input) {
		input.oninput = function () {
			keyFilter[container.id] = input.value;
			applyKeyFilter(container);
		};
	}
	applyKeyFilter(container);
	// Belt and braces: make every combo box really show its stored value, no
	// matter how the browser handled the selected attribute above.
	Array.prototype.forEach.call(container.querySelectorAll('select[data-value]'), function (sel) {
		var want = sel.getAttribute('data-value');
		if (want && sel.value !== want) {
			sel.value = want;
		}
	});
	container.onclick = function (ev) {
		var b = ev.target.closest('button');
		if (!b) { return; }
		if (b.getAttribute('data-pin') !== null) {
			var pidx = parseInt(b.getAttribute('data-pin'), 10);
			var pk = keys[pidx];
			var want = !pins[pinId(path, section, pk.key)];
			fetch('/api/pins', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', 'X-Editor-Request': '1' },
				body: JSON.stringify({ path: path, section: section, key: pk.key, pinned: want })
			}).then(function (r) { return r.json(); }).then(function (res) {
				if (!res.ok) {
					toast(res.error || 'pin failed', true);
					return;
				}
				if (res.pinned) { pins[pinId(path, section, pk.key)] = true; }
				else { delete pins[pinId(path, section, pk.key)]; }
				var c = lastRender[container.id];
				if (c) { renderKeys(container, c.path, c.section, c.keys, c.reload, c.plain); }
			}).catch(function (e) { toast(String(e), true); });
			return;
		}
		var idx = parseInt(b.getAttribute('data-save') !== null ? b.getAttribute('data-save') : b.getAttribute('data-del'), 10);
		var k = keys[idx];
		var value = el('kv-' + idx).value;
		var remove = b.getAttribute('data-del') !== null;
		post({ path: path, section: section, key: k.key, value: value, remove: remove }).then(function (res) {
			if (res.ok) {
				var what = (remove ? 'Removed ' : 'Saved ') + k.key;
				if (res.applied) {
					// Pushed into the running engine. A warning means the value
					// is live but part of it could not take effect (an
					// unloaded font index keeps the old typeface).
					toast(what + (res.applyWarning ? ' — applied, but ' + res.applyWarning : ' — applied'),
						!!res.applyWarning);
				} else if (res.applyError) {
					// The engine has no field for this key, or cannot rebuild
					// the screen, so a restart would not help either; the file
					// still has the value.
					toast(what + ' — not applied: ' + res.applyError, true);
				} else if (res.needsRestart) {
					// Read once while the script boots into a structure nothing
					// rebuilds: only a restart picks it up.
					toast(what + ' — saved, restart the game to apply'
						+ (res.applyReason ? ': ' + res.applyReason : ''), true);
				} else if (res.needsReload) {
					// Written to the file; the running engine picks it up on
					// "Reload motif in engine" (or a restart).
					toast(what + ' — saved, reload the motif to apply'
						+ (res.applyReason ? ': ' + res.applyReason : ''), true);
				} else {
					toast(what);
				}
				if (reload) { reload(); }
			} else {
				toast(res.error || 'save failed', true);
			}
		}).catch(function (e) { toast(String(e), true); });
	};
}
/* ---- motif view ---- */
function loadMotif() {
	api('/api/motif').then(function (data) {
		state.motif = data;
		el('motif-hint').innerHTML = 'Motif: <b>' + esc(data.path || '(not found)') + '</b> &middot; '
			+ (data.sections || []).length + ' sections &middot; values come from the motif file, type/default shown on hover';
		renderTree();
		if (!selName && state.motif.tree && state.motif.tree.length) {
			selName = null;
			selectFirst();
		} else if (selName) {
			selectByName(selName);
		}
	}).catch(function (e) { toast(String(e), true); });
}
// reloadMotif re-reads the motif file into the running engine, so edits to
// reload-only keys (backgrounds, [Music], [Files] paths, ...) show up without
// restarting the game. Refused with 409 while a match runs.
function reloadMotif() {
	fetch('/api/reload', {
		method: 'POST',
		headers: { 'X-Editor-Request': '1' }
	}).then(function (r) { return r.json(); }).then(function (res) {
		if (res.ok) {
			toast('Motif reloaded');
			loadMotif();
		} else {
			toast(res.error || 'reload failed', true);
		}
	}).catch(function (e) { toast(String(e), true); });
}
function nodeCount(n) {
	return n.total ? n.defined + '/' + n.total : (n.keys || []).length;
}
// treeHTML renders section nodes as the collapsible tree the motif and the
// stage view share. A node id is "<prefix><i>" and a child "<id>.<j>", so the
// index stays in step with the data-sid attributes in the DOM.
function treeHTML(nodes, prefix, index) {
	var h = ['<ul>'];
	(nodes || []).forEach(function (n, i) {
		var sid = prefix + i;
		index[sid] = n;
		var label = '<span class="t">' + esc(n.name || n.title) + '</span>'
			+ '<span class="small">' + nodeCount(n) + '</span>';
		h.push('<li>');
		if (n.children && n.children.length) {
			h.push('<details open><summary data-sid="' + sid + '">' + label + '</summary>'
				+ treeHTML(n.children, sid + '.', index) + '</details>');
		} else {
			h.push('<div class="row" data-sid="' + sid + '">' + label + '</div>');
		}
		h.push('</li>');
	});
	h.push('</ul>');
	return h.join('');
}
// Motif sections as a tree: one node per screen, background layers under their
// BGdef and each [Begin Action n] under the layer that plays it.
function renderTree() {
	treeIndex = {};
	var h = [];
	(state.motif.tree || []).forEach(function (g, gi) {
		// A screen with no section of its own still shows up, empty.
		// Every group needs its own id prefix: sharing one makes the groups
		// overwrite each other in treeIndex, so clicking a row would show
		// whichever group wrote that id last.
		h.push('<details class="grp" open><summary>' + esc(g.label) + '</summary>'
			+ treeHTML(g.sections, 'g' + gi + '-', treeIndex) + '</details>');
	});
	var box = el('motif-sections');
	box.innerHTML = h.join('');
	box.onclick = function (ev) {
		var t = ev.target.closest('[data-sid]');
		if (!t) { return; }
		showSection(t.getAttribute('data-sid'));
	};
}
function selectByName(name) {
	for (var sid in treeIndex) {
		if (treeIndex[sid].name === name) {
			showSection(sid);
			return true;
		}
	}
	return false;
}
function selectFirst() {
	// treeIndex is filled in tree order, so the first entry with keys is the
	// first section worth editing. Falling back to the first entry at all keeps
	// a group whose sections have no keys selectable.
	var first = null;
	for (var sid in treeIndex) {
		if (!first) { first = sid; }
		if (treeIndex[sid].keys && treeIndex[sid].keys.length) {
			showSection(sid);
			return;
		}
	}
	if (first) { showSection(first); }
}
function showSection(sid) {
	var n = treeIndex[sid];
	if (!n) { return; }
	selName = n.name;
	Array.prototype.forEach.call(el('motif-sections').querySelectorAll('[data-sid]'), function (x) {
		x.classList.toggle('sel', x.getAttribute('data-sid') === sid);
	});
	el('motif-title').textContent = n.name
		+ (n.title && n.title !== n.name ? '  [' + n.title + ']' : '')
		+ (n.field ? '  struct field: ' + n.field : '')
		+ (n.runtime ? '  (parsed at load time)' : '')
		+ '  (' + (n.kind || 'section') + ')';
	if (!n.keys || !n.keys.length) {
		// No keys: show the whole section content instead of an empty table.
		el('motif-keys').innerHTML = n.raw
			? rawBlock(n.raw)
			: '<div class="small">this entry only groups the sections below it</div>';
		return;
	}
	renderKeys(el('motif-keys'), state.motif.path, n.name, n.keys, loadMotif);
}
// rawBlock renders a section body as preformatted, read only content.
function rawBlock(text) {
	return '<pre class="raw">' + esc(text) + '</pre>';
}
/* ---- generic def file browser (Stage / Character tabs) ---- */
function editorEls(id) {
	return {
		id: id,
		hint: el(id + '-hint'),
		title: el(id + '-title'),
		sections: el(id + '-sections'),
		keys: el(id + '-keys')
	};
}
function loadFile(path, els) {
	api('/api/file?path=' + encodeURIComponent(path)).then(function (data) {
		if (!data.ok) {
			els.sections.innerHTML = '';
			els.title.textContent = '';
			els.keys.innerHTML = '<div class="small">' + esc(data.error || 'load failed') + '</div>';
			return;
		}
		state.page = { path: data.path, sections: data.sections || [] };
		els.hint.innerHTML = 'File: <b>' + esc(data.path) + '</b>';
		// Both .def views read like the motif view: a section tree.
		renderFileTree(els);
	}).catch(function (e) { toast(String(e), true); });
}

/* ---- .def views (Stage / Character) ---- */
// bgBlockOf returns the background block a .def section belongs to. A stage
// .def has one "bg" block: its [BGdef] definition, its [BG n] layers and the
// [bgctrldef] / [bgctrl] / [bgctrl3d] controllers. A motif has one per
// <name>BGdef. Anything else ([Info], [PlayerInfo], ...) is no block.
function bgBlockOf(name) {
	var head = String(name || '').trim().toLowerCase().split(/[ \t]/)[0];
	var block = head.replace(/(ctrl3d|ctrldef|ctrl|def)$/, '');
	if (/^bg[0-9]*$/.test(block)) { return 'bg'; }
	if (block !== 'bg' && /bg$/.test(block)) { return block; }
	return '';
}
function headOf(name) {
	return String(name || '').trim().toLowerCase().split(/[ \t]/)[0];
}
// actionNoOf returns the number of a [Begin Action n] style section, 0 if it is
// not one.
function actionNoOf(name) {
	var m = /^(?:begin|loop|end)[ _]?action[ _]?([0-9]+)$/i.exec(String(name || '').trim());
	return m ? parseInt(m[1], 10) : 0;
}
// buildFileTree nests the sections of a .def the way the motif tree nests them:
// one node per section, a background block owning its layers and controllers,
// and each [Begin Action n] under the layer that plays it (actionno = n).
function buildFileTree(sections) {
	var roots = [], owners = {}, defs = {}, defNames = {}, pending = [], lastBG = null, playing = {};
	var nodeOf = function (s) {
		return {
			name: s.name || '(top level)', title: s.title || s.name,
			keys: s.keys || [], raw: s.raw || '', children: []
		};
	};
	// The owner appears in the tree where its first layer or controller is, so
	// the file order is kept everywhere else.
	var ownerFor = function (block) {
		if (owners[block]) { return owners[block]; }
		var node = defs[block];
		if (!node) {
			node = { name: block + 'def', title: block + 'def', kind: 'bgdef', children: [] };
		}
		owners[block] = node;
		roots.push(node);
		lastBG = node;
		return node;
	};
	// A "<block>def" section is the definition of a background block, so it is
	// the node the layers and controllers hang under.
	(sections || []).forEach(function (s) {
		var name = s.name || '(top level)';
		var head = headOf(name);
		if (head.slice(-3) !== 'def' || !bgBlockOf(head)) { return; }
		var node = nodeOf(s);
		node.kind = 'bgdef';
		defs[head.slice(0, -3)] = node;
		defNames[name] = node;
	});
	(sections || []).forEach(function (s) {
		var name = s.name || '(top level)';
		if (defNames[name]) { return; }  // already placed as a block owner
		var node = nodeOf(s);
		var block = bgBlockOf(name);
		if (block) {
			var owner = ownerFor(block);
			node.kind = /ctrl/.test(headOf(name)) ? 'bgctrl' : 'layer';
			owner.children.push(node);
			if (node.kind === 'layer') {
				// An action no layer claims hangs under the last block seen.
				lastBG = owner;
				(node.keys || []).forEach(function (k) {
					if (String(k.key).toLowerCase() === 'actionno' && /^[0-9]+$/.test(String(k.value).trim())) {
						var num = parseInt(String(k.value).trim(), 10);
						if (!playing[num]) { playing[num] = node; }
					}
				});
			}
			return;
		}
		var num = actionNoOf(name);
		if (num) {
			node.kind = 'action';
			if (playing[num]) {
				playing[num].children.push(node);
			} else {
				// The layer that plays it may come later in the file, so the
				// action is parked until every layer has been seen.
				pending.push({ node: node, num: num });
			}
			return;
		}
		node.kind = 'section';
		roots.push(node);
	});
	// Attach the parked actions now that every layer is known.
	pending.forEach(function (p) {
		var owner = playing[p.num] || lastBG;
		if (owner) {
			owner.children.push(p.node);
		} else {
			roots.push(p.node);
		}
	});
	return roots;
}
function renderFileTree(els) {
	var view = viewState(els.id);
	view.index = {};
	els.sections.innerHTML = treeHTML(buildFileTree(state.page.sections), 'n', view.index);
	els.sections.onclick = function (ev) {
		var t = ev.target.closest('[data-sid]');
		if (!t) { return; }
		showFileSection(t.getAttribute('data-sid'), els);
	};
	// Keep the selected section across a reload, the way the motif view does.
	var sid, wanted = null;
	for (sid in view.index) {
		if (view.sel && view.index[sid].name === view.sel) { wanted = sid; break; }
	}
	if (!wanted) {
		for (sid in view.index) {
			if (view.index[sid].keys && view.index[sid].keys.length) { wanted = sid; break; }
		}
	}
	showFileSection(wanted || 'n0', els);
}
function showFileSection(sid, els) {
	var view = viewState(els.id);
	var n = view.index[sid];
	if (!n) { return; }
	view.sel = n.name;
	Array.prototype.forEach.call(els.sections.querySelectorAll('[data-sid]'), function (x) {
		x.classList.toggle('sel', x.getAttribute('data-sid') === sid);
	});
	els.title.textContent = n.name + '  (' + (n.kind || 'section') + ')';
	if (!n.keys || !n.keys.length) {
		els.keys.innerHTML = n.raw
			? rawBlock(n.raw)
			: '<div class="small">this entry only groups the sections below it</div>';
		return;
	}
	var keys = n.keys.map(function (k) { return { key: k.key, value: k.value, defined: true, choices: k.choices }; });
	// A .def has no schema behind its keys, so the State column would only
	// repeat itself here.
	renderKeys(els.keys, state.page.path, n.name, keys, function () { loadFile(state.page.path, els); }, true);
}
// defCombo fills a view's combo box with one "label — def" entry per file and
// hands the chosen one to onPick. The stage and the character view share it, so
// picking a file reads the same in both.
function defCombo(listId, items, onPick) {
	var list = el(listId);
	if (!items.length) {
		list.innerHTML = '<option value="">none found</option>';
		list.disabled = true;
		return;
	}
	var h = [];
	items.forEach(function (it, i) {
		h.push('<option value="' + i + '">' + esc(it.label) + '  \u2014  ' + esc(it.def)
			+ (it.available ? '' : '  (missing)') + '</option>');
	});
	list.innerHTML = h.join('');
	list.disabled = false;
	list.onchange = function () { onPick(items[parseInt(this.value, 10)]); };
	list.value = '0';
	onPick(items[0]);
}
function loadStages() {
	api('/api/stages').then(function (data) {
		if (!data.ok) {
			el('stage-hint').textContent = data.error || 'unable to read select.def';
			return;
		}
		el('stage-hint').innerHTML = 'select.def: <b>' + esc(data.selectDef) + '</b>';
		// One combo box entry per stage: the name the file gives it and the
		// .def behind it, so both are visible without opening the file.
		defCombo('stage-list', (data.stages || []).map(function (s) {
			return { label: s.name || s.def, def: s.def, path: s.path || s.def, available: s.available };
		}), function (st) {
			if (!st) { return; }
			viewState('stage').sel = null;
			el('stage-title').textContent = st.def;
			// The stage sprite file sits next to the .def, so preview that one.
			previewDef(sffEls('stage'), st.path);
			loadFile(st.path, editorEls('stage'));
		});
	}).catch(function (e) { toast(String(e), true); });
}

/* ---- character view ---- */
function loadCharacters() {
	api('/api/characters').then(function (data) {
		if (!data.ok) {
			el('character-hint').textContent = data.error || 'unable to read select.def';
			return;
		}
		el('character-hint').innerHTML = 'select.def: <b>' + esc(data.selectDef) + '</b>';
		// The same combo box as the stage view, showing the display name the
		// character declares and the .def behind it.
		defCombo('character-list', (data.characters || []).map(function (c) {
			var info = c.info || {};
			var who = info.name || c.def;
			return {
				label: who + (info.author ? ' by ' + info.author : ''),
				def: c.def,
				path: c.path || c.def,
				available: c.available,
				title: who
					+ (info.author ? ' by ' + info.author : '')
					+ (c.stages && c.stages.length ? '  stages: ' + c.stages.join(', ') : '')
			};
		}), function (c) {
			if (!c) { return; }
			viewState('character').sel = null;
			el('character-title').textContent = c.title;
			// The sprite file sits next to the .def, so preview that one.
			previewDef(sffEls('character'), c.path);
			loadFile(c.path, editorEls('character'));
		});
	}).catch(function (e) { toast(String(e), true); });
}

/* ---- sprite preview (Stage / Character) ---- */
// sffURL builds the /api/sff URL for one sprite. The service answers with a PNG,
// which is all an <img> needs.
function sffURL(file, group, number) {
	return '/api/sff?file=' + encodeURIComponent(file)
		+ '&group=' + encodeURIComponent(group)
		+ '&number=' + encodeURIComponent(number);
}
// sffEls collects one view's preview elements, the same way editorEls does.
function sffEls(id) {
	return {
		file: el(id + '-sff-file'),
		group: el(id + '-sff-group'),
		number: el(id + '-sff-number'),
		img: el(id + '-sff-img'),
		hint: el(id + '-sff-hint')
	};
}
// showSff points a view's preview at one sprite and reports what happened.
function showSff(els) {
	var file = els.file.value.trim();
	var group = els.group.value.trim() || '0';
	var number = els.number.value.trim() || '0';
	if (!file) {
		els.hint.textContent = 'no .sff file given';
		els.img.removeAttribute('src');
		return;
	}
	els.hint.textContent = file + '  ' + group + ',' + number;
	els.img.onerror = function () {
		els.hint.textContent = 'no sprite ' + group + ',' + number + ' in ' + file;
	};
	els.img.src = sffURL(file, group, number);
}
// sffForDef guesses the .sff next to a .def, so the preview starts on the
// sprite file of the character or stage being edited.
function sffForDef(path) {
	if (!/\.def$/i.test(path)) { return ''; }
	return path.replace(/\.def$/i, '.sff');
}
// previewDef points a view's preview at the .sff beside the given .def.
function previewDef(els, path) {
	var sff = sffForDef(path);
	if (!sff) { return; }
	els.file.value = sff;
	showSff(els);
}
function initSff() {
	['stage', 'character'].forEach(function (id) {
		var els = sffEls(id);
		el(id + '-sff-show').onclick = function () { showSff(els); };
		[els.file, els.group, els.number].forEach(function (input) {
			input.onkeydown = function (ev) { if (ev.key === 'Enter') { showSff(els); } };
		});
	});
}

function init() {
	Array.prototype.forEach.call(document.querySelectorAll('nav button'), function (b) {
		b.onclick = function () { setView(b.getAttribute('data-view')); };
	});
	api('/api/status').then(function (s) {
		el('status').textContent = 'Ikemen GO ' + s.version + '  ' + s.platform
			+ '  port ' + s.port + (s.serviceFromCommandLine ? ' (-httpservice)' : '');
	}).catch(function () { el('status').textContent = 'offline'; });
	initSff();
	el('motif-reload').onclick = reloadMotif;
	// Pins first: the tables read them while rendering.
	loadPins().then(function () {
		loadMotif();
		var m = /[?&]view=([a-z]+)/.exec(window.location.search);
		setView(m ? m[1] : 'motif');
	});
}
if (document.readyState === 'loading') {
	document.addEventListener('DOMContentLoaded', init);
} else {
	init();
}
</script>
</body>
</html>
`
