package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdcolor "image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/ini.v1"
)

// -------------------------------------------------------------------
// Structural breakdown of the motif (reflection over src/motif.go)
// -------------------------------------------------------------------

func editorSchemaHasKey(s editorSchemaSectionJSON, key string) bool {
	for _, k := range s.Keys {
		if k.Key == key {
			return true
		}
	}
	return false
}

func editorSchemaHasDynamicKey(s editorSchemaSectionJSON, prefix string) bool {
	for _, k := range s.Keys {
		if k.Dynamic && k.Key == prefix+".<name>" {
			return true
		}
	}
	return false
}

func TestEditorMotifStructure(t *testing.T) {
	structure := editorMotifStructure()
	if len(structure) == 0 {
		t.Fatal("editorMotifStructure() returned no sections")
	}
	bySection := map[string]editorSchemaSectionJSON{}
	for _, s := range structure {
		bySection[s.Section] = s
	}
	info, ok := bySection["info"]
	if !ok {
		t.Fatal("expected an [Info] section in the motif structure")
	}
	if !editorSchemaHasKey(info, "localcoord") {
		t.Error("[Info] is missing the localcoord key")
	}
	title, ok := bySection["title_info"]
	if !ok {
		t.Fatal("expected a [Title Info] section in the motif structure")
	}
	// Nested struct fields must be flattened into dotted keys.
	for _, key := range []string{"fadein.time", "menu.pos", "menu.item.font", "loading.text", "footer.title.font"} {
		if !editorSchemaHasKey(title, key) {
			t.Errorf("[Title Info] is missing the %q key", key)
		}
	}
	if !editorSchemaHasDynamicKey(title, "menu.itemname") {
		t.Error("[Title Info] is missing the dynamic menu.itemname key")
	}
}

// -------------------------------------------------------------------
// select.def breakdown
// -------------------------------------------------------------------

func TestEditorSelectDefSections(t *testing.T) {
	text := strings.Join([]string{
		"[Characters]",
		"; a comment only line",
		"kfm, stages/robby.def, stages/robby2.def, order = 1, ai = 8",
		"empty",
		"[ExtraStages]",
		"stages/robby.def, hidden = 0",
		"[Options]",
		"arcade.maxmatches = 10",
		"[StoryMode]",
		"arc1.name = Arc 1",
	}, "\n")
	characters, stages := editorSelectDefSections(text)

	if len(characters) != 2 {
		t.Fatalf("characters = %d, want 2", len(characters))
	}
	if characters[0].Def != "kfm" {
		t.Errorf("characters[0].Def = %q, want %q", characters[0].Def, "kfm")
	}
	if len(characters[0].Stages) != 2 {
		t.Errorf("characters[0].Stages = %v, want 2 entries", characters[0].Stages)
	}
	if characters[0].Params["ai"] != "8" || characters[0].Params["order"] != "1" {
		t.Errorf("characters[0].Params = %v, want ai=8 and order=1", characters[0].Params)
	}
	if characters[1].Def != "empty" {
		t.Errorf("characters[1].Def = %q, want %q", characters[1].Def, "empty")
	}

	if len(stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(stages))
	}
	if stages[0].Def != "stages/robby.def" {
		t.Errorf("stages[0].Def = %q, want %q", stages[0].Def, "stages/robby.def")
	}
	if stages[0].Params["hidden"] != "0" {
		t.Errorf("stages[0].Params = %v, want hidden=0", stages[0].Params)
	}
}

// -------------------------------------------------------------------
// Writing values back into a .def file
// -------------------------------------------------------------------

func TestEditorEditINIFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.def")
	initial := "[Info]\nname = Test ; keep me\nlocalcoord = 320, 240\n\n[Files]\nselect = select.def\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	value := "Renamed"
	if err := editorEditINIFile(path, "Info", "name", &value); err != nil {
		t.Fatalf("update: %v", err)
	}
	file := "other.def"
	if err := editorEditINIFile(path, "Info", "file", &file); err != nil {
		t.Fatalf("add: %v", err)
	}
	size := "5"
	if err := editorEditINIFile(path, "Other", "size", &size); err != nil {
		t.Fatalf("add section: %v", err)
	}
	if err := editorEditINIFile(path, "Files", "select", nil); err != nil {
		t.Fatalf("remove: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"name = Renamed ; keep me", // comment preserved
		"localcoord = 320, 240",
		"file = other.def", // key added to an existing section
		"[Other]",          // section created on demand
		"size = 5",
		"[Files]", // untouched section header
	} {
		if !strings.Contains(text, want) {
			t.Errorf("edited file is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "select =") {
		t.Errorf("removed key is still present:\n%s", text)
	}
}

func TestEditorEditINIFileKeepsCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crlf.def")
	if err := os.WriteFile(path, []byte("[Info]\r\nname = Test\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	value := "Renamed"
	if err := editorEditINIFile(path, "Info", "name", &value); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "name = Renamed\r\n") {
		t.Errorf("CRLF line endings were not preserved: %q", text)
	}
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Errorf("mixed line endings: %q", text)
	}
}

// -------------------------------------------------------------------
// Path sandboxing
// -------------------------------------------------------------------

func TestEditorSandboxPath(t *testing.T) {
	oldBase := sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.baseDir = oldBase }()

	if _, err := editorSandboxPath("../escape.def"); err == nil {
		t.Error("expected a path outside the game folder to be rejected")
	}
	if _, err := editorSandboxPath(""); err == nil {
		t.Error("expected an empty path to be rejected")
	}
	p, err := editorSandboxPath("data/ikemen1/system.def")
	if err != nil {
		t.Fatalf("relative path rejected: %v", err)
	}
	if want := filepath.Join(dir, "data", "ikemen1", "system.def"); p != want {
		t.Errorf("editorSandboxPath() = %v, want %v", p, want)
	}
}

// -------------------------------------------------------------------
// HTTP handlers
// -------------------------------------------------------------------

func TestEditorKeyChoices(t *testing.T) {
	// Enumerated keys resolve on the last segment of a dotted key.
	if got := editorChoicesFor("", "Title Info", "menu.item.active.bg.default.projection"); len(got) != 3 {
		t.Errorf("editorChoicesFor(projection) = %v, want the 3 projection values", got)
	}
	if got := editorChoicesFor("", "Begin Action", "TRANS"); len(got) != 7 {
		t.Errorf("editorChoicesFor(TRANS) = %v, want the 7 trans values", got)
	}
	if got := editorChoicesFor("", "Dialogue Info", "p1.face.textwrap"); len(got) != 3 {
		t.Errorf("editorChoicesFor(p1.face.textwrap) = %v, want 3 values", got)
	}
	// "type" depends on the section: background layers, background controllers
	// and font definitions all spell it differently.
	if got := editorChoicesFor("", "TitleBG Background Sky", "type"); len(got) != 5 {
		t.Errorf("motif background type = %v, want the 5 background values", got)
	}
	if got := editorChoicesFor("stages/stage1.def", "BG 1", "type"); len(got) != 5 {
		t.Errorf("stage [BG n] type = %v, want the 5 background values", got)
	}
	if got := editorChoicesFor("", "VersusBG StageInfo", "type"); len(got) != 12 {
		t.Errorf("bg controller type = %v, want the 12 controller values", got)
	}
	if got := editorChoicesFor("stages/stage1.def", "StageInfo", "type"); len(got) != 12 {
		t.Errorf("stage [StageInfo] type = %v, want the 12 controller values", got)
	}
	if got := editorChoicesFor("font/Open_Sans.def", "Def", "type"); len(got) != 2 {
		t.Errorf("font type = %v, want truetype/bitmap", got)
	}
	if got := editorChoicesFor("", "Files", "font1.type"); len(got) != 2 {
		t.Errorf("motif [Files] font type = %v, want truetype/bitmap", got)
	}
	// Free form keys must stay text fields.
	if got := editorChoicesFor("", "Title Info", "menu.pos"); got != nil {
		t.Errorf("editorChoicesFor(menu.pos) = %v, want nil", got)
	}
	if got := editorChoicesFor("", "Info", "type"); got != nil {
		t.Errorf("editorChoicesFor(unknown type) = %v, want nil", got)
	}
	if got := editorChoicesFor("", "", ""); got != nil {
		t.Errorf("editorChoicesFor(empty) = %v, want nil", got)
	}
}

func TestEditorDynamicKeyMatch(t *testing.T) {
	cases := []struct {
		schema, key string
		want        bool
	}{
		// Plain map, written as a flattened dotted key.
		{"menu.itemname.<name>", "menu.itemname.editor", true},
		{"menu.itemname.<name>", "menu.pos", false},
		// Bare map key inside the section ([Files] font<n>).
		{"files.font.<name>", "font1", true},
		{"files.font.<name>", "spr", false},
		// keyfirst text map: "<prefix>.<name>.<field>".
		{"textinput.text.<name>", "textinput.aspectheight.text", true},
		{"textinput.text.<name>", "textinput.overlay.col", false},
	}
	for _, c := range cases {
		if got := editorDynamicKeyMatch(c.schema, c.key); got != c.want {
			t.Errorf("editorDynamicKeyMatch(%q, %q) = %v, want %v", c.schema, c.key, got, c.want)
		}
	}
}

func TestEditorDynamicMapKeys(t *testing.T) {
	// The dynamic entry name decides which file keys belong to it.
	structure := editorMotifStructure()
	bySection := map[string]editorSchemaSectionJSON{}
	for _, s := range structure {
		bySection[s.Section] = s
	}
	find := func(section, prefix string) *editorSchemaKeyJSON {
		s, ok := bySection[section]
		if !ok {
			return nil
		}
		for i, k := range s.Keys {
			if k.Dynamic && strings.HasPrefix(k.Key, prefix) {
				return &s.Keys[i]
			}
		}
		return nil
	}
	// A pattern keyed map continues at the current level of the key path.
	if k := find("select_info", "cell.<name>"); k == nil {
		t.Errorf("[Select Info] is missing the cell.<name> dynamic key")
	} else if k.Key != "cell.<name>" {
		t.Errorf("cell map key = %q, want %q", k.Key, "cell.<name>")
	}
	// A "map:" map takes its name from the lua tag, so [Files] font<n> keys
	// are recognized and are not reported as unknown.
	if k := find("files", "font"); k == nil {
		t.Error("[Files] is missing the font map key")
	}
	// Plain map fields keep their ini tag name.
	if k := find("title_info", "menu.itemname.<name>"); k == nil {
		t.Error("[Title Info] is missing the menu.itemname.<name> dynamic key")
	}
}

// findGroup / findNode walk the tree built by editorMotifTree.
func findGroup(groups []*editorGroupJSON, label string) *editorGroupJSON {
	for _, g := range groups {
		if g.Label == label {
			return g
		}
	}
	return nil
}

// findTop looks a section up among the direct children only.
func findTop(nodes []*editorSectionNodeJSON, name string) *editorSectionNodeJSON {
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
	}
	return nil
}

func findNode(nodes []*editorSectionNodeJSON, name string) *editorSectionNodeJSON {
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
		if c := findNode(n.Children, name); c != nil {
			return c
		}
	}
	return nil
}

func childNames(n *editorSectionNodeJSON) []string {
	names := []string{}
	for _, c := range n.Children {
		names = append(names, c.Name)
	}
	return names
}

func TestEditorMotifTree(t *testing.T) {
	sections := []editorMotifSectionJSON{
		{Name: "Title Info", Title: "title_info", Field: "TitleInfo", Defined: 2, Total: 3},
		{Name: "TitleBGdef", Title: "titlebgdef", Field: "TitleBgDef"},
		{Name: "TitleBG Background Sky", Title: "TitleBG Background Sky", Defined: 2, Total: 2,
			Keys: []editorMotifKeyJSON{{Key: "type", Value: "normal", Defined: true}}},
		{Name: "TitleBG Background Clouds Top", Title: "TitleBG Background Clouds Top"},
		{Name: "Victory Screen", Title: "victory_screen", Field: "VictoryScreen"},
		{Name: "VictoryBGdef", Title: "victorybgdef", Field: "VictoryBgDef"},
		{Name: "VictoryBG Background Sky", Title: "VictoryBG Background Sky"},
		{Name: "VictoryBG Background Clouds Top", Title: "VictoryBG Background Clouds Top"},
		{Name: "VictoryBG Background Clouds Bottom", Title: "VictoryBG Background Clouds Bottom"},
		{Name: "VictoryBG Text", Title: "VictoryBG Text", Defined: 1, Total: 1,
			Keys: []editorMotifKeyJSON{{Key: "actionno", Value: "203", Defined: true}}},
		{Name: "Begin Action 203", Title: "Begin Action 203", Defined: 1, Total: 1,
			Keys: []editorMotifKeyJSON{{Key: "0", Value: "anim 203, 1", Defined: true}}},
	}
	tree := editorMotifTree(sections)

	title := findGroup(tree, "Title")
	if title == nil {
		t.Fatal("the tree has no Title group")
	}
	if findNode(title.Sections, "Title Info") == nil {
		t.Error("Title group is missing [Title Info]")
	}
	bg := findNode(title.Sections, "TitleBGdef")
	if bg == nil {
		t.Fatal("Title group is missing [TitleBGdef]")
	}
	want := []string{"TitleBG Background Sky", "TitleBG Background Clouds Top"}
	if got := childNames(bg); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("TitleBGdef children = %v, want %v", got, want)
	}

	victory := findGroup(tree, "Victory")
	if victory == nil {
		t.Fatal("the tree has no Victory group")
	}
	if findNode(victory.Sections, "Victory Screen") == nil {
		t.Error("Victory group is missing [Victory Screen]")
	}
	vbg := findNode(victory.Sections, "VictoryBGdef")
	if vbg == nil {
		t.Fatal("Victory group is missing [VictoryBGdef]")
	}
	if got := len(vbg.Children); got != 4 {
		t.Errorf("VictoryBGdef has %d layers, want 4", got)
	}
	// The action belongs to the layer that plays it (actionno = 203).
	layer := findNode(vbg.Children, "VictoryBG Text")
	if layer == nil {
		t.Fatal("VictoryBG Text is not nested under VictoryBGdef")
	}
	if got := childNames(layer); len(got) != 1 || got[0] != "Begin Action 203" {
		t.Errorf("VictoryBG Text children = %v, want [Begin Action 203]", got)
	}
	// And it must not be listed at the group level as well.
	if findTop(victory.Sections, "Begin Action 203") != nil {
		t.Error("[Begin Action 203] is also listed at the Victory group level")
	}
}

// A screen group without any section must not marshal to a null array, which
// the page would then try to iterate.
func TestEditorMotifTreeEmptyGroups(t *testing.T) {
	tree := editorMotifTree([]editorMotifSectionJSON{
		{Name: "Title Info", Title: "title_info", Field: "TitleInfo", Defined: 1, Total: 2},
	})
	pause := findGroup(tree, "Pause")
	if pause == nil {
		t.Fatal("the tree has no Pause group")
	}
	if pause.Sections == nil {
		t.Error("an empty group has a nil Sections slice (marshals to null)")
	}
	data, err := json.Marshal(pause)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("empty group marshals with a null: %s", data)
	}
	// Every declared screen is present, whether it has sections or not.
	if len(tree) != len(editorScreenGroups)+1 {
		t.Errorf("tree has %d groups, want %d", len(tree), len(editorScreenGroups)+1)
	}
}

// The Motif struct does not declare sections in the order a .def writes them
// (Music comes before Info), so the breakdown has to follow the file.
func TestEditorMotifSectionOrder(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, text string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The same order as a real motif: Info, Files, Music, then the screens.
	mustWrite("save/config.ini", "[Config]\nMotif = data/system.def\n")
	mustWrite("data/system.def", strings.Join([]string{
		"[Info]", "name = Test", "localcoord = 320, 240",
		"[Files]", "spr = f.sff", "snd = f.snd",
		"[Music]", "select.bgm = sound/Select.mp3", "select.bgm.loop = 1",
		"[Title Info]", "fadein.time = 10",
		"[TitleBGdef]", "bgclearcolor = 255, 255, 255",
		"[TitleBG Background Sky]", "type = normal",
		"[Music ]", "", // trailing whitespace is trimmed by the parser
	}, "\n")+"\n")

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	oldBase, oldFlags := sys.baseDir, sys.cmdFlags
	defer func() {
		_ = os.Chdir(oldwd)
		sys.baseDir, sys.cmdFlags = oldBase, oldFlags
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	sys.baseDir = "./"
	sys.cmdFlags = map[string]string{"-config": "save/config.ini"}

	sections := editorMotifSections()
	got := []string{}
	for _, s := range sections {
		got = append(got, s.Name)
	}
	want := []string{"Info", "Files", "Music", "Title Info", "TitleBGdef"}
	if len(got) < len(want) {
		t.Fatalf("sections = %v, want at least %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("section %d = %q, want %q (all: %v)", i, got[i], w, got)
		}
	}

	// The tree keeps the same order inside the screen groups.
	motif := findGroup(editorMotifTree(sections), "Motif")
	if motif == nil {
		t.Fatal("the tree has no Motif group")
	}
	order := []string{}
	for _, n := range motif.Sections {
		order = append(order, n.Name)
	}
	for i, w := range []string{"Info", "Files", "Music"} {
		if i >= len(order) || order[i] != w {
			t.Errorf("Motif group entry %d = %q, want %q (all: %v)", i, safeAt(order, i), w, order)
		}
	}
}

func safeAt(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// Sections whose body is not key = value ([Begin Action n], [Infobox Text],
// ...) have no keys to show, so their content is returned raw.
func TestEditorRawSections(t *testing.T) {
	text := strings.Join([]string{
		"[Info]", "name = Test",
		"[Begin Action 203]", "", "202,1, 35,0, 1", "202,1, 30,0, 1", "",
		"[Infobox Text]", "First line.", "Second line.", "",
		"[Title Info]", "fadein.time = 10",
	}, "\n") + "\n"
	headers, bodies := editorRawSections(text)
	for norm, want := range map[string]string{
		"title_info":       "Title Info",
		"begin_action_203": "Begin Action 203",
		"infobox_text":     "Infobox Text",
	} {
		if headers[norm] != want {
			t.Errorf("header[%q] = %q, want %q", norm, headers[norm], want)
		}
	}
	// Free form bodies keep every line, without the blank lines around them.
	if got := bodies["begin_action_203"]; got != "202,1, 35,0, 1\n202,1, 30,0, 1" {
		t.Errorf("Begin Action body = %q", got)
	}
	if got := bodies["infobox_text"]; got != "First line.\nSecond line." {
		t.Errorf("Infobox Text body = %q", got)
	}
	if got := bodies["title_info"]; got != "fadein.time = 10" {
		t.Errorf("Title Info body = %q", got)
	}
	// A trailing comment block belongs to the next section's header, not here.
	withComments := strings.Join([]string{
		"[Begin Action 110]", "110,1, 0, 0, 1", "", ";---------", ";Next screen", "",
		"[Title Info]", "fadein.time = 10",
	}, "\n") + "\n"
	_, more := editorRawSections(withComments)
	if got := more["begin_action_110"]; got != "110,1, 0, 0, 1" {
		t.Errorf("body with trailing comments = %q, want only the data lines", got)
	}
}

func TestEditorRawSectionsThroughAPI(t *testing.T) {
	// The raw body has to reach the JSON of a section with no keys.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "system.def"), []byte(
		"[Info]\nname = Test\n\n[Title Info]\nfadein.time = 10\nmenu.pos = 1,2\n\n[Begin Action 110]\n110,1, 0, 0, 1\n110,1, 10, 0, 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldwd, _ := os.Getwd()
	oldBase, oldFlags := sys.baseDir, sys.cmdFlags
	defer func() {
		_ = os.Chdir(oldwd)
		sys.baseDir, sys.cmdFlags = oldBase, oldFlags
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	sys.baseDir = "./"
	sys.cmdFlags = map[string]string{"-r": "data/system.def", "-config": "save/config.ini"}

	sections := editorMotifSections()
	seen := map[string]bool{}
	for _, s := range sections {
		if !seen[s.Name] && s.Name == "Title Info" {
			seen[s.Name] = true
			// A section that has keys shows them in the table, never a duplicate
			// dump of the whole body.
			if len(s.Keys) == 0 {
				t.Fatalf("the Title Info section has no keys")
			}
			if s.Raw != "" {
				t.Errorf("a keyed section must not carry raw content, got %q", s.Raw)
			}
		}
	}
	for _, s := range sections {
		if s.Name != "Begin Action 110" {
			continue
		}
		if len(s.Keys) != 0 {
			t.Fatalf("Begin Action section has %d keys, want none", len(s.Keys))
		}
		if s.Raw != "110,1, 0, 0, 1\n110,1, 10, 0, 1" {
			t.Errorf("raw content = %q", s.Raw)
		}
		return
	}
	t.Fatal("the Begin Action 110 section is missing from the breakdown")
}

func TestEditorSectionHeaders(t *testing.T) {
	// go-ini lowercases section names, so the original spelling is recovered
	// from the raw text.
	text := "; comment\n[Title Info]\nmenu.pos = 1\n[TitleBG Background Sky]\ntype = normal\n[Begin Action 203]\n0, 0, 0, 10\n[Infobox Text]\nbody\n"
	headers := editorSectionHeaders(text)
	for norm, want := range map[string]string{
		"title_info":             "Title Info",
		"titlebg_background_sky": "TitleBG Background Sky",
		"begin_action_203":       "Begin Action 203",
		"infobox_text":           "Infobox Text",
	} {
		if got := headers[norm]; got != want {
			t.Errorf("editorSectionHeaders[%q] = %q, want %q", norm, got, want)
		}
	}
}

func TestEditorActionNo(t *testing.T) {
	// Section names reach the tree normalized ("Begin Action 203" ->
	// "begin_action_203"), which is what this parser sees.
	cases := map[string]int{
		"begin_action_203": 203,
		"begin_action_110": 110,
		"loop_action_5":    5,
		"end_action_7":     7,
		"beginaction_9":    9,
		"endaction_7":      7,
	}
	for norm, want := range cases {
		got, ok := editorActionNo(norm)
		if !ok || got != want {
			t.Errorf("editorActionNo(%q) = %v, %v; want %v, true", norm, got, ok, want)
		}
	}
	for _, norm := range []string{"titlebg_background_sky", "begin_action", "beginaction_x"} {
		if _, ok := editorActionNo(norm); ok {
			t.Errorf("editorActionNo(%q) matched a non action section", norm)
		}
	}
}

func TestEditorSchemaForMapField(t *testing.T) {
	// Sections whose name comes from the file (results screens, pause menus)
	// are map fields of the Motif struct; their element struct supplies keys.
	structure := editorMotifStructure()
	kinds := map[string]editorSchemaSectionJSON{}
	for _, s := range structure {
		kinds[s.Field] = s
	}
	for _, field := range []string{"ResultsScreen", "PauseMenu"} {
		sch, ok := kinds[field]
		if !ok {
			t.Errorf("the motif structure has no entry for the %s map", field)
			continue
		}
		if sch.Kind != "map" {
			t.Errorf("%s kind = %q, want map", field, sch.Kind)
		}
		if sch.Pattern == "" {
			t.Errorf("%s has no section pattern", field)
		}
		if len(sch.Keys) == 0 {
			t.Errorf("%s has no keys", field)
		}
	}
	if m, ok := kinds["Music"]; !ok {
		t.Error("the motif structure has no [Music] entry")
	} else if m.Kind != "dynamic" || len(m.Keys) != 8 {
		t.Errorf("Music entry = kind %q with %d keys, want dynamic with 8", m.Kind, len(m.Keys))
	}
}

func TestEditorIsRuntimeSection(t *testing.T) {
	for _, name := range []string{"TitleBG Background Sky", "VersusBG 5", "TitleBG StageInfo", "ContinueBG Text",
		"TitleBGctrl Fade In", "TitleBGctrldef", "BG0Ctrl Credits"} {
		if !editorIsRuntimeSection(name) {
			t.Errorf("editorIsRuntimeSection(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"Title Info", "Music", "Begin Action 203", "BG 1", "Infobox Text"} {
		if editorIsRuntimeSection(name) {
			t.Errorf("editorIsRuntimeSection(%q) = true, want false", name)
		}
	}
}

func TestEditorRuntimeSchemaCoversParserKeys(t *testing.T) {
	// The keys readBackGround / bgCtrl.read look for. When the engine gains a
	// new key this test fails, so the schema has to be updated with it.
	bg := editorBGElementSchema()
	haveBG := map[string]bool{}
	for _, k := range bg.Keys {
		haveBG[k.Key] = true
	}
	for _, want := range []string{"type", "layerno", "actionno", "spriteno", "start", "delta", "tile",
		"window", "maskwindow", "trans", "alpha", "velocity", "sin.x", "sin.y", "id", "shader",
		"projection", "scalemode", "scalefilter", "path", "loop", "width", "xscale", "roundpos"} {
		if !haveBG[want] {
			t.Errorf("the BG layer schema is missing the %q key", want)
		}
	}
	if !editorSchemaHasDynamicKey(bg, "shaderparam") {
		t.Error("the BG layer schema is missing the shaderparam keys")
	}
	ctrl := editorBGCtrlSchema()
	haveCtrl := map[string]bool{}
	for _, k := range ctrl.Keys {
		haveCtrl[k.Key] = true
	}
	for _, want := range []string{"type", "time", "ctrlid", "value", "x", "y", "source", "dest",
		"add", "mul", "sinadd", "sincolor", "color", "hue", "sctrlid"} {
		if !haveCtrl[want] {
			t.Errorf("the BG controller schema is missing the %q key", want)
		}
	}
	// A layer section gets the layer keys, a controller the controller ones.
	if sch := editorRuntimeSchema("TitleBG Background Sky"); sch == nil || len(sch.Keys) != len(bg.Keys) {
		t.Error("a BG layer section does not resolve to the layer schema")
	}
	if sch := editorRuntimeSchema("TitleBGctrl Fade In"); sch == nil || len(sch.Keys) != len(ctrl.Keys) {
		t.Error("a BG controller section does not resolve to the controller schema")
	}
	if sch := editorRuntimeSchema("Title Info"); sch != nil {
		t.Error("a struct backed section must not resolve to a runtime schema")
	}
}

func TestEditorBuildRuntimeSectionHasTypes(t *testing.T) {
	// A BGdef layer is parsed at load time by readBackGround, so its keys have a
	// type and a default just like the ones the Motif struct declares.
	section := editorBuildRuntimeSection("TitleBG Background Sky", "TitleBG Background Sky", map[string]string{
		"type":     "normal",
		"layerno":  "2",
		"spriteno": "1, 0",
	})
	if !section.Runtime {
		t.Error("runtime section is not marked as such")
	}
	byKey := map[string]editorMotifKeyJSON{}
	for _, k := range section.Keys {
		byKey[k.Key] = k
	}
	if k := byKey["layerno"]; k.Type != "int32" || k.Default != "0" || !k.Defined || k.Value != "2" {
		t.Errorf("layerno = %+v, want int32 = 0 with the file value 2", k)
	}
	if k := byKey["start"]; k.Type != "[2]float32" || k.Defined {
		t.Errorf("start = %+v, want a [2]float32 key that the file does not define", k)
	}
	// mask starts at -1 (newAnimation inside newBackGround), not blank.
	if k := byKey["mask"]; k.Type != "int32" || k.Default != "-1" {
		t.Errorf("mask = %+v, want int32 = -1", k)
	}
	// A key the file has that the parser does not read is reported as unknown.
	if k := byKey["bogus"]; k.Key != "" {
		t.Error("an unexpected key showed up in a runtime section")
	}
	section = editorBuildRuntimeSection("TitleBG Background Sky", "TitleBG Background Sky", map[string]string{
		"type": "normal", "notakey": "1",
	})
	for _, k := range section.Keys {
		if k.Key == "notakey" && !k.Unknown {
			t.Error("a key the BG parser does not read is not reported as unknown")
		}
	}
	// The enumerated keys of a layer still resolve to their combo box values.
	if got := editorChoicesFor("", "TitleBG Background Sky", "type"); len(got) != 5 {
		t.Errorf("type choices = %v, want the five BG types", got)
	}
	if got := editorChoicesFor("", "TitleBGctrl Fade", "type"); len(got) != 12 {
		t.Errorf("controller type choices = %v, want the twelve BG controller types", got)
	}
}

// Every el('id') and editorEls('id') call in the page must match an element
// that really exists, otherwise the view throws on null.
func TestEditorKeyStateOrder(t *testing.T) {
	// Rows are grouped by state: what the file sets, then what falls back to the
	// engine default, then what the engine does not read at all. Inside a group
	// the order the schema declares is kept.
	sch := &editorSchemaSectionJSON{
		Section: "info", Title: "Info", Keys: []editorSchemaKeyJSON{
			{Key: "name", Type: "string"},
			{Key: "author", Type: "string"},
			{Key: "localcoord", Type: "[2]int32", Default: "320, 240"},
			{Key: "version", Type: "string", Default: "0, 1"},
			{Key: "showfps", Type: "int32", Default: "0"},
		},
	}
	section := editorBuildSection("Info", "info", "Info", sch, map[string]string{
		"localcoord": "640, 480",
		"author":     "Someone",
		"bogus":      "1",
		"als bogus":  "2",
	})
	got := []string{}
	for _, k := range section.Keys {
		got = append(got, k.Key)
	}
	// defined (author, localcoord) first, then the missing ones in the order the
	// schema declares them, then the unknown ones the file adds.
	want := []string{"author", "localcoord", "name", "version", "showfps", "als bogus", "bogus"}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("key %d = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
	// The counters do not depend on the order.
	if section.Defined != 4 || section.Total != len(want) {
		t.Errorf("defined/total = %d/%d, want 4/%d", section.Defined, section.Total, len(want))
	}
	// The three states rank in display order.
	if editorKeyStateRank(editorMotifKeyJSON{Defined: true}) != 0 ||
		editorKeyStateRank(editorMotifKeyJSON{Defined: false}) != 1 ||
		editorKeyStateRank(editorMotifKeyJSON{Defined: true, Unknown: true}) != 2 {
		t.Error("the state ranks are not defined < missing < unknown")
	}
}

func TestEditorPageElementIDs(t *testing.T) {
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(editorPageHTML, -1) {
		ids[m[1]] = true
	}
	if len(ids) == 0 {
		t.Fatal("the page has no element ids")
	}
	// Direct lookups: el('x') and editorEls('x') (which builds <x>-hint, â€¦).
	// The leading group keeps identifiers like setModel('x') from matching.
	direct := regexp.MustCompile(`(?:^|[^A-Za-z0-9_$])(el|editorEls)\('([a-z-]+)'\)`)
	for _, m := range direct.FindAllStringSubmatch(editorPageHTML, -1) {
		if m[1] != "el" {
			continue
		}
		if !ids[m[2]] {
			t.Errorf("el(%q) has no matching element id in the page", m[2])
		}
	}
	// Composite lookups: editorEls('x') builds the ids the view really uses.
	// Every .def view shows its sections as a tree, so they share one extra id
	// on top of the three common ones.
	viewExtra := map[string]string{
		"stage":     "-sections",
		"character": "-sections",
	}
	for _, m := range direct.FindAllStringSubmatch(editorPageHTML, -1) {
		if m[1] != "editorEls" {
			continue
		}
		suffixes := []string{"-hint", "-title", "-keys"}
		if extra, ok := viewExtra[m[2]]; ok {
			suffixes = append(suffixes, extra)
		}
		for _, suffix := range suffixes {
			if !ids[m[2]+suffix] {
				t.Errorf("editorEls(%q) builds %q, which the page does not have", m[2], m[2]+suffix)
			}
		}
	}
	// The dynamic ids built by renderKeys and loadFile, plus the sprite
	// preview each .def view owns.
	preview := []string{"-sff-file", "-sff-group", "-sff-number", "-sff-img", "-sff-show", "-sff-hint"}
	for _, id := range []string{"motif-sections", "motif-keys", "motif-title",
		"motif-reload",
		"stage-list", "stage-sections", "stage-keys", "stage-title", "stage-hint",
		"character-list", "character-sections", "character-keys", "character-title", "character-hint"} {
		if !ids[id] {
			t.Errorf("the page is missing the element id %q", id)
		}
	}
	for _, view := range []string{"stage", "character"} {
		for _, suffix := range preview {
			if !ids[view+suffix] {
				t.Errorf("the page is missing the element id %q", view+suffix)
			}
		}
	}
	// Both .def views pick their file from a combo box and list the sections as
	// a tree, the motif way, and the old list / picker markup is gone.
	for _, view := range []string{"stage", "character"} {
		if !strings.Contains(editorPageHTML, `<select id="`+view+`-list"`) {
			t.Errorf("the %s view does not use a combo box to pick the file", view)
		}
		if !strings.Contains(editorPageHTML, `<div class="sectree" id="`+view+`-sections">`) {
			t.Errorf("the %s view does not render a section tree", view)
		}
	}
	if strings.Contains(editorPageHTML, `id="character-pick"`) || strings.Contains(editorPageHTML, `ul class="list"`) {
		t.Error("the old character list and section picker markup is still in the page")
	}
	// Both views share one tree renderer and one combo box builder, so they
	// cannot drift apart.
	if strings.Count(editorPageHTML, "function treeHTML(") != 1 {
		t.Error("the shared tree renderer is missing or duplicated")
	}
	if n := strings.Count(editorPageHTML, "defCombo("); n != 3 {
		t.Errorf("defCombo is called %d times, want the definition plus one per .def view", n)
	}
	// A .def has no schema behind its keys, so the shared .def key table drops
	// the State column; the motif view keeps all four.
	plainCalls := 0
	for _, line := range strings.Split(editorPageHTML, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "renderKeys(") && strings.HasSuffix(line, ", true);") {
			plainCalls++
		}
	}
	if plainCalls != 1 {
		t.Errorf("%d renderKeys calls drop the schema columns, want only the shared .def one", plainCalls)
	}
	// Type and default are tooltips, not a column: the header has no
	// "Type / default" cell, and the key cell plus the value control carry
	// the type/default as a title attribute.
	if strings.Contains(editorPageHTML, "Type / default</th>") {
		t.Error("the key table still has a Type / default column")
	}
	if !strings.Contains(editorPageHTML, `'<td class="k"'`) || !strings.Contains(editorPageHTML, `title="`) {
		t.Error("the key cells do not carry a tooltip")
	}
}

func TestEditorHandleFileAndSave(t *testing.T) {
	oldBase := sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.baseDir = oldBase }()
	if err := os.WriteFile(filepath.Join(dir, "x.def"), []byte("[Info]\nname = A\n\n[Begin Action]\ntrans = add\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	editorHandleFile(rr, httptest.NewRequest(http.MethodGet, "/api/file?path=x.def", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/file = %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "\"name\"") {
		t.Errorf("GET /api/file did not return the keys: %s", rr.Body.String())
	}
	// Enumerated keys must expose their values for the combo box.
	if !strings.Contains(rr.Body.String(), "\"choices\"") || !strings.Contains(rr.Body.String(), "addalpha") {
		t.Errorf("GET /api/file did not expose the trans choices: %s", rr.Body.String())
	}

	body := `{"path":"x.def","section":"Info","key":"name","value":"B"}`

	// Writes require the custom header so cross-site requests are blocked.
	rr = httptest.NewRecorder()
	editorHandleSave(rr, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body)))
	if rr.Code != http.StatusForbidden {
		t.Errorf("POST /api/save without X-Editor-Request = %d, want 403", rr.Code)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body))
	req.Header.Set("X-Editor-Request", "1")
	editorHandleSave(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/save = %d: %s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "x.def"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name = B") {
		t.Errorf("value was not written: %q", data)
	}

	// Paths outside the game folder are refused.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/save",
		strings.NewReader(`{"path":"../escape.def","section":"Info","key":"name","value":"C"}`))
	req.Header.Set("X-Editor-Request", "1")
	editorHandleSave(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("POST /api/save outside the game folder = %d, want 400", rr.Code)
	}
}

// Re-saving a value that did not change must leave the file byte for byte
// identical. The engine's own motif aligns its inline comments with runs of
// spaces, and a save used to collapse those to a single space, so a no-op edit
// produced a diff.
func TestEditorSaveKeepsInlineCommentSpacing(t *testing.T) {
	oldBase := sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.baseDir = oldBase }()

	const text = "[Title Info]\n" +
		"menu.boxcursor.visible = 0         ;Set to 1 to enable default cursor display\n" +
		"menu.window.visibleitems = 6\t;how many items fit\n"
	path := filepath.Join(dir, "x.def")
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}

	save := func(key, value string) {
		t.Helper()
		body := fmt.Sprintf(`{"path":"x.def","section":"Title Info","key":%q,"value":%q}`, key, value)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body))
		req.Header.Set("X-Editor-Request", "1")
		editorHandleSave(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("POST /api/save %s = %d: %s", key, rr.Code, rr.Body.String())
		}
	}

	// Saving the value the file already has, with the value the file has
	// (including its spaces), must not disturb the alignment.
	save("menu.boxcursor.visible", "0")
	save("menu.window.visibleitems", "6")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != text {
		t.Errorf("a no-op save rewrote the file:\n got %q\nwant %q", got, text)
	}

	// A real change keeps the comment and its alignment, only the value moves.
	save("menu.boxcursor.visible", "1")
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(text, "visible = 0 ", "visible = 1 ", 1)
	if string(got) != want {
		t.Errorf("after a real save:\n got %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// Motif reload over HTTP (POST /api/reload)
// -------------------------------------------------------------------

func TestEditorMotifReloadBlocked(t *testing.T) {
	oldRunning := sys.gameRunning
	oldEnd, oldPost := sys.fightLoopEnd, sys.postMatchFlg
	oldNet, oldReplay := sys.netConnection, sys.replayFile
	oldRollback := sys.rollback.session
	oldLoader := sys.loader.state
	defer func() {
		sys.gameRunning = oldRunning
		sys.fightLoopEnd, sys.postMatchFlg = oldEnd, oldPost
		sys.netConnection, sys.replayFile = oldNet, oldReplay
		sys.rollback.session = oldRollback
		sys.loader.state = oldLoader
	}()

	quiet := func() {
		sys.gameRunning = false
		sys.fightLoopEnd, sys.postMatchFlg = true, false
		sys.netConnection, sys.replayFile = nil, nil
		sys.rollback.session = nil
		sys.loader.state = LS_Complete
	}

	// A quiet menu state allows a reload.
	quiet()
	if reason := editorMotifReloadBlocked(); reason != "" {
		t.Errorf("a quiet engine blocks a reload: %q", reason)
	}

	// Anything that owns frame state blocks it.
	for _, c := range []struct {
		name  string
		setup func()
	}{
		{"match flag", func() { sys.gameRunning = true }},
		{"mid-match frames", func() { sys.gameRunning = true; sys.fightLoopEnd = false }},
		{"netplay", func() { sys.netConnection = &NetConnection{} }},
		{"replay", func() { sys.replayFile = &ReplayFile{} }},
		{"rollback", func() { sys.rollback.session = &RollbackSession{} }},
		{"loading", func() { sys.loader.state = LS_Loading }},
	} {
		quiet()
		c.setup()
		if reason := editorMotifReloadBlocked(); reason == "" {
			t.Errorf("%s does not block a reload", c.name)
		}
	}
}

func TestEditorHandleReloadGuards(t *testing.T) {
	oldRunning := sys.gameRunning
	oldEnd, oldPost := sys.fightLoopEnd, sys.postMatchFlg
	oldNet, oldReplay := sys.netConnection, sys.replayFile
	oldRollback := sys.rollback.session
	oldLoader := sys.loader.state
	defer func() {
		sys.gameRunning = oldRunning
		sys.fightLoopEnd, sys.postMatchFlg = oldEnd, oldPost
		sys.netConnection, sys.replayFile = oldNet, oldReplay
		sys.rollback.session = oldRollback
		sys.loader.state = oldLoader
	}()

	reload := func() *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/reload", nil)
		req.Header.Set("X-Editor-Request", "1")
		editorHandleReload(rr, req)
		return rr
	}

	// Reloads require the custom header so cross-site requests are blocked.
	rr := httptest.NewRecorder()
	editorHandleReload(rr, httptest.NewRequest(http.MethodPost, "/api/reload", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("POST /api/reload without X-Editor-Request = %d, want 403", rr.Code)
	}
	// Only POST is accepted.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/reload", nil)
	req.Header.Set("X-Editor-Request", "1")
	editorHandleReload(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/reload = %d, want 405", rr.Code)
	}

	// The handler runs the reload on the engine thread; stand in for
	// System.await, which drains that queue in the game.
	stop := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case f := <-sys.mainThreadTask:
				f()
			case <-stop:
				return
			}
		}
	}()
	defer func() {
		close(stop)
		<-drained
	}()

	// A running match is refused with 409, and nothing is reloaded.
	sys.gameRunning = true
	sys.fightLoopEnd, sys.postMatchFlg = true, false
	sys.netConnection, sys.replayFile = nil, nil
	sys.rollback.session = nil
	sys.loader.state = LS_Complete
	if rr := reload(); rr.Code != http.StatusConflict {
		t.Errorf("POST /api/reload during a match = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	sys.gameRunning = false

	// Without a configured motif there is nothing to reload.
	sys.gameRunning = false
	sys.fightLoopEnd, sys.postMatchFlg = true, false
	sys.netConnection, sys.replayFile = nil, nil
	sys.rollback.session = nil
	sys.loader.state = LS_Complete
	oldBase := sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.baseDir = oldBase }()
	if rr := reload(); rr.Code != http.StatusNotFound {
		t.Errorf("POST /api/reload without a motif = %d, want 404: %s", rr.Code, rr.Body.String())
	}

	// With a motif but no Lua state (unit tests never boot the script), the
	// reload cannot hand the table to anyone.
	oldFlags := sys.cmdFlags
	sys.cmdFlags = map[string]string{}
	defer func() { sys.cmdFlags = oldFlags }()
	motifPath := filepath.Join(sys.baseDir, "m.def")
	if err := os.WriteFile(motifPath, []byte("[Info]\nname = M\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sys.cmdFlags["-r"] = motifPath
	if rr := reload(); rr.Code != http.StatusInternalServerError {
		t.Errorf("POST /api/reload without Lua = %d, want 500: %s", rr.Code, rr.Body.String())
	}
}

func TestEditorKeyFilterMarkup(t *testing.T) {
	// Every key table carries a filter box that narrows the rows by the KEY
	// column. It filters live (oninput) and hides non-matching rows.
	if !strings.Contains(editorPageHTML, `class="keyfilter"`) ||
		!strings.Contains(editorPageHTML, `class="keyfilter-input"`) {
		t.Error("the key table has no filter box")
	}
	if !strings.Contains(editorPageHTML, "function applyKeyFilter(") {
		t.Error("applyKeyFilter() is missing from the page")
	}
	if !strings.Contains(editorPageHTML, "keyFilter[container.id] = input.value") {
		t.Error("the filter box is not wired to the live filter")
	}
	if !strings.Contains(editorPageHTML, "tr.classList.toggle('hidden', !hit)") {
		t.Error("applyKeyFilter does not hide non-matching rows")
	}
	// The match is against the KEY cell, and the filter text survives a
	// re-render (a save reloads the section).
	if !strings.Contains(editorPageHTML, "tr.querySelector('td.k')") {
		t.Error("the filter does not read the KEY column")
	}
	if !strings.Contains(editorPageHTML, "value=\"' + escAttr(keyFilter[container.id] || '') + '\"") {
		t.Error("the filter box does not restore its previous text")
	}
	// "*" is a wildcard for any run ("font*offset"); without it the match is
	// a plain case-insensitive substring.
	if !strings.Contains(editorPageHTML, "function keyFilterMatch(") {
		t.Error("keyFilterMatch() is missing from the page")
	}
	if !strings.Contains(editorPageHTML, "q.split('*')") {
		t.Error("the filter does not support * wildcards")
	}
}

func TestEditorReloadMarkup(t *testing.T) {
	// The motif view offers a reload button that posts to the reload endpoint
	// and re-renders the breakdown afterwards.
	if !strings.Contains(editorPageHTML, `id="motif-reload"`) {
		t.Error("the motif view has no reload button")
	}
	if !strings.Contains(editorPageHTML, "function reloadMotif()") {
		t.Error("the reloadMotif() function is missing from the page")
	}
	if !strings.Contains(editorPageHTML, "'/api/reload'") {
		t.Error("the page never posts to /api/reload")
	}
}

// -------------------------------------------------------------------
// Live apply (motif only)
// -------------------------------------------------------------------

func TestEditorMotifKeyNeedsReload(t *testing.T) {
	// Keys the load time derives something from cannot be reproduced by
	// assigning the field, so they must ask for a reload.
	for _, c := range []struct{ section, key string }{
		{"TitleBGdef", "layerno"},
		{"victorybgdef", "type"},
		{"Info", "localcoord"},
		{"Music", "title.bgm"},
		{"Files", "spr"},
		{"files", "snd"},
		{"Files", "fight"},
		{"Files", "model"},
		{"Files", "glyphs"},
		{"Files", "logo.storyboard"},
		{"Files", "intro.storyboard"},
		{"Files", "font0.file"},
		{"Files", "font12.height"},
	} {
		reload, reason := editorMotifKeyNeedsReload(c.section, c.key)
		if !reload {
			t.Errorf("%v.%v applies live, want a reload", c.section, c.key)
		} else if reason == "" {
			t.Errorf("%v.%v asks for a reload without a reason", c.section, c.key)
		}
	}
	// Values the script reads once into a structure nothing rebuilds can only be
	// picked up by a restart, so they must not claim a reload would apply them.
	for _, c := range []struct{ section, key string }{
		{"Files", "select"},
		{"Files", "module"},
	} {
		if mode, reason := editorMotifApplyClassify(c.section, c.key); mode != editorApplyRestart {
			t.Errorf("%v.%v is %v (%v), want a restart", c.section, c.key, mode, reason)
		}
	}
	// Plain values go straight into the struct: their declaring struct holds no
	// load time snapshot, so drawing reads the field itself.
	for _, c := range []struct{ section, key string }{
		{"Files", "font0"},
		{"Info", "name"},
		{"Info", "author"},
	} {
		if reload, reason := editorMotifKeyNeedsReload(c.section, c.key); reload {
			t.Errorf("%v.%v asks for a reload (%v), want a live apply", c.section, c.key, reason)
		}
	}
	// A value the draw path reads through a load time snapshot is refreshed
	// rather than reported as a plain live apply: the field is assigned and the
	// screen's snapshots are rebuilt, so the next draw shows it.
	for _, c := range []struct{ section, key string }{
		{"Title Info", "menu.item.active.font"},
		{"Title Info", "menu.item.font"},
		{"Title Info", "footer.title.text"},
		{"Option Info", "menu.item.active.font"},
	} {
		if mode, _ := editorMotifApplyClassify(c.section, c.key); mode != editorApplyRefresh {
			t.Errorf("%v.%v is %v, want a refresh", c.section, c.key, mode)
		}
	}
	// A key under a user named section has no single field to scope a rebuild
	// to, so it has to be a reload.
	if mode, reason := editorMotifApplyClassify("Survival Results Screen", "winstext.text"); mode != editorApplyReload {
		t.Errorf("a map backed section is %v (%v), want a reload", mode, reason)
	}
}

func TestEditorIsMotifPath(t *testing.T) {
	oldBase, oldFlags := sys.baseDir, sys.cmdFlags
	dir := t.TempDir()
	sys.baseDir = dir
	sys.cmdFlags = map[string]string{"-r": filepath.Join(dir, "system.def")}
	defer func() { sys.baseDir, sys.cmdFlags = oldBase, oldFlags }()
	if err := os.WriteFile(filepath.Join(dir, "system.def"), []byte("[Info]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !editorIsMotifPath("system.def") {
		t.Error("the motif path was not recognized")
	}
	if editorIsMotifPath("chars/kfm/kfm.def") {
		t.Error("a character .def was taken for the motif")
	}
}

// editorTestMotif builds a Motif the way loadMotif does, including the case
// insensitive section matching the engine relies on.
func editorTestMotif(t *testing.T, text string) *Motif {
	t.Helper()
	f, err := ini.LoadSources(editorINIOptions(), []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return &Motif{IniFile: f}
}

// A reload-only key must still be recorded in the in-memory copy, so a later
// Motif.Save cannot write the old value back over the editor's edit, and the
// struct must be left alone.
func TestEditorApplyMotifValueReloadOnly(t *testing.T) {
	m := editorTestMotif(t, "[Files]\nspr = old.sff\n")
	if err := editorApplyMotifValue(m, "Files", "spr", "new.sff", false, editorApplyReload); err != nil {
		t.Fatal(err)
	}
	if got := editorINIKeyValue(editorTestSection(t, m, "Files"), "spr"); got != "new.sff" {
		t.Errorf("in-memory spr = %q, want new.sff", got)
	}
	if m.Files.Spr != "" {
		t.Errorf("the struct changed for a reload-only key: spr = %q", m.Files.Spr)
	}
}

// A plain key is assigned to the struct as well as the in-memory file.
func TestEditorApplyMotifValueLive(t *testing.T) {
	m := editorTestMotif(t, "[Info]\nname = Old\n")
	if err := editorApplyMotifValue(m, "Info", "name", "New", false, editorApplyLive); err != nil {
		t.Fatal(err)
	}
	if m.Info.Name != "New" {
		t.Errorf("Info.Name = %q, want New", m.Info.Name)
	}
	if got := editorINIKeyValue(editorTestSection(t, m, "Info"), "name"); got != "New" {
		t.Errorf("in-memory name = %q, want New", got)
	}
}

// Removing a key drops it from the in-memory copy too.
func TestEditorApplyMotifValueRemove(t *testing.T) {
	m := editorTestMotif(t, "[Info]\nname = Old\n")
	if err := editorApplyMotifValue(m, "info", "NAME", "", true, editorApplyReload); err != nil {
		t.Fatal(err)
	}
	if got := editorINIKeyValue(editorTestSection(t, m, "Info"), "name"); got != "" {
		t.Errorf("name is still in memory: %q", got)
	}
}

func TestEditorApplyMotifValueGuards(t *testing.T) {
	if err := editorApplyMotifValue(nil, "Info", "name", "x", false, editorApplyLive); err == nil {
		t.Error("a nil motif was accepted")
	}
	if err := editorApplyMotifValue(&Motif{}, "Info", "name", "x", false, editorApplyLive); err == nil {
		t.Error("a motif without an in-memory copy was accepted")
	}
}

// A background definition key is not a path the reflection walk knows, but the
// in-memory copy must still be updated so a later Motif.Save cannot undo it.
func TestEditorApplyMotifValueFallbackKey(t *testing.T) {
	m := editorTestMotif(t, "[TitleBGdef]\nlayerno = 0\n")
	if err := editorApplyMotifValue(m, "TitleBGdef", "time", "30", false, editorApplyReload); err != nil {
		t.Fatal(err)
	}
	if got := editorINIKeyValue(editorTestSection(t, m, "TitleBGdef"), "time"); got != "30" {
		t.Errorf("in-memory time = %q, want 30", got)
	}
	// An unknown section is created rather than lost.
	m2 := editorTestMotif(t, "[Info]\nname = x\n")
	if err := editorApplyMotifValue(m2, "NewThing", "k", "v", false, editorApplyReload); err != nil {
		t.Fatal(err)
	}
	if got := editorINIKeyValue(editorTestSection(t, m2, "NewThing"), "k"); got != "v" {
		t.Errorf("in-memory k = %q, want v", got)
	}
}

// A file spells a section for the reader ("[Title Info]") while the struct
// field is tagged with the engine's name ("title_info"), so the query has to
// be built the way loadMotif builds it.
func TestEditorMotifQuery(t *testing.T) {
	for _, c := range []struct{ section, key, want string }{
		{"Title Info", "menu.item.active.font", "title_info.menu.item.active.font"},
		{"Info", "name", "info.name"},
		{"VS Screen", "time", "vs_screen.time"},
		{"Demo Mode", "enabled", "demo_mode.enabled"},
		{"Files", "font0.file", "files.font0.file"},
		{"  Info  ", "  Name ", "info.name"},
		// A map backed section is matched by its own name, and the map regex
		// spans the separator, so the underscored spelling still resolves.
		{"Survival Results Screen", "textwrap", "survival_results_screen.textwrap"},
	} {
		if got := editorMotifQuery(c.section, c.key); got != c.want {
			t.Errorf("editorMotifQuery(%q, %q) = %q, want %q", c.section, c.key, got, c.want)
		}
	}
}

func TestEditorApplyMotifValueSpacedSection(t *testing.T) {
	// The reported failure: [Title Info] menu.item.active.font.
	m := editorTestMotif(t, "[Title Info]\nmenu.item.active.font = 1\n")
	if err := editorApplyMotifValue(m, "Title Info", "menu.item.active.font", "2", false, editorApplyLive); err != nil {
		t.Fatalf("a spaced section was rejected: %v", err)
	}
	if got := editorINIKeyValue(editorTestSection(t, m, "Title Info"), "menu.item.active.font"); got != "2" {
		t.Errorf("in-memory font = %q, want 2", got)
	}
}

// A refreshable key assigns the field and rebuilds the screen's snapshots, so
// the TextSprite the screen draws is the one built from the new value.
func TestEditorApplyMotifValueRefresh(t *testing.T) {
	oldMotif := sys.motif
	oldBase := sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.item.active.font = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	// A stand in for the loaded motif: the snapshots are the fields the draw
	// path reads, and PopulateDataPointers fills them only while they are nil.
	// The SFF has to be there because SetAnim resolves sprite animations from it.
	m := &Motif{IniFile: f, Sff: newSff()}
	sys.motif = *m

	query := editorMotifQuery("Title Info", "menu.item.active.font")
	if mode, _ := editorMotifApplyClassify("Title Info", "menu.item.active.font"); mode != editorApplyRefresh {
		t.Skipf("this key is classified %v, not refreshable here", mode)
	}
	// A snapshot that already exists, as it would after the motif has loaded. The
	// Lua system script is handed a handle to this object, so the refresh has to
	// update it rather than hand back a new one.
	scope := editorRefreshMotifScope(m, query)
	if !scope.IsValid() {
		t.Fatal("the scope of the edited key could not be resolved")
	}
	m.populateDataPointers()
	m.applyPostParsePosAdjustments()
	handle := m.TitleInfo.Menu.Item.Active.TextSpriteData
	if handle == nil {
		t.Fatal("the motif was not populated with a snapshot")
	}
	if err := editorApplyMotifValue(m, "Title Info", "menu.item.active.font", "2", false, editorApplyRefresh); err != nil {
		t.Fatal(err)
	}
	if m.TitleInfo.Menu.Item.Active.Font[0] != 2 {
		t.Errorf("Font[0] = %v, want the assigned 2", m.TitleInfo.Menu.Item.Active.Font[0])
	}
	if got := editorINIKeyValue(editorTestSection(t, m, "Title Info"), "menu.item.active.font"); got != "2" {
		t.Errorf("in-memory font = %q, want 2", got)
	}
	// The handle the script holds must still be the object being drawn.
	if m.TitleInfo.Menu.Item.Active.TextSpriteData != handle {
		t.Error("the refresh replaced the TextSprite, so the Lua script keeps drawing the old one")
	}
	// A refill must not clobber the properties the edit did not touch: the
	// object is rebuilt from the struct, so a field the refill failed to resolve
	// would silently fall back to a default and break the menu.
	if handle.text != m.TitleInfo.Menu.Item.Active.Text {
		t.Errorf("text = %q, want %q", handle.text, m.TitleInfo.Menu.Item.Active.Text)
	}
	if handle.textInit != handle.text {
		t.Errorf("textInit %q drifted from text %q", handle.textInit, handle.text)
	}
	if handle.layerno != m.TitleInfo.Menu.Item.Active.Layerno {
		t.Errorf("layerno = %v, want %v", handle.layerno, m.TitleInfo.Menu.Item.Active.Layerno)
	}
	if handle.bank != m.TitleInfo.Menu.Item.Active.Font[1] {
		t.Errorf("bank = %v, want the assigned %v", handle.bank, m.TitleInfo.Menu.Item.Active.Font[1])
	}
	if handle.fnt != nil && handle.bank == 0 {
		t.Log("font 0 resolved to a real font, as it should")
	}
	// The position pass adds Menu.Pos to offsetInit, so re-applying must not walk
	// the sprite further. The refill restores the pristine offset first, which is
	// what makes a repeated apply land in the same place.
	once := handle.offsetInit
	if err := editorApplyMotifValue(m, "Title Info", "menu.item.active.font", "3", false, editorApplyRefresh); err != nil {
		t.Fatal(err)
	}
	if m.TitleInfo.Menu.Item.Active.TextSpriteData != handle {
		t.Error("re-running the position pass replaced the snapshot")
	}
	if once != handle.offsetInit {
		t.Errorf("a repeated apply drifted the sprite: %v then %v", once, handle.offsetInit)
	}
}

// A [Select Info] title is a TextMapProperties: its text lives in a mode keyed
// map (title.text.arcade, ...) the Lua script copies into the TextSprite when a
// mode is picked (main.t_itemname: textImgSetText). Unlike the other text
// properties, whose Text is a Go string, there is no single string for the
// refill to read, so refreshing the snapshot after a title.offset / title.font
// edit used to blank the sprite and make the title disappear until the mode was
// picked again. The refill must keep the text while still applying the edit.
func TestEditorSelectInfoTitleRefreshKeepsText(t *testing.T) {
	oldMotif := sys.motif
	oldBase := sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Select Info]\n"+
		"title.offset = 159, 19\n"+
		"title.font = f-6x9.def, 0, 0, 255, 255, 255, 255, -1\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f, Sff: newSff()}
	sys.motif = *m
	m.populateDataPointers()
	m.applyPostParsePosAdjustments()

	ts := m.SelectInfo.Title.TextSpriteData
	if ts == nil {
		t.Fatal("the [Select Info] title was not populated with a TextSprite")
	}
	// Stand in for the mode pick: the script sets the sprite's text.
	// textImgSetText only touches text, so textInit keeps its load value.
	ts.text = "ARCADE"
	textInit := ts.textInit
	beforeOffset := ts.offsetInit

	for _, c := range []struct{ key, value string }{
		{"title.offset", "100, 50"},
		{"title.font", "1, 2, 1, 10, 20, 30, 255, -1"},
	} {
		mode, reason := editorMotifApplyClassify("Select Info", c.key)
		if mode != editorApplyRefresh {
			t.Errorf("%s is classified %v (%v), want it refreshed live", c.key, mode, reason)
		}
		if err := editorApplyMotifValue(m, "Select Info", c.key, c.value, false, mode); err != nil {
			t.Fatalf("saving %s: %v", c.key, err)
		}
		if ts.text != "ARCADE" {
			t.Errorf("after saving %s the title text is %q, want the mode text to survive", c.key, ts.text)
		}
		if ts.textInit != textInit {
			t.Errorf("after saving %s textInit is %q, want %q", c.key, ts.textInit, textInit)
		}
	}
	// The edits still reached the snapshot: the refill is not simply skipped.
	if ts.offsetInit == beforeOffset {
		t.Error("title.offset did not move the title TextSprite")
	}
	if want := [2]float32{100, 50}; ts.offsetInit != want {
		t.Errorf("title.offset = %v, want %v", ts.offsetInit, want)
	}
	if ts.bank != 2 {
		t.Errorf("title.font bank = %v, want the saved 2", ts.bank)
	}
}

// The [Select Info] title is not the only map-backed text: record, the menu
// items (ItemProperties) and the text input all keep their text in a mode keyed
// map the script fills at runtime. The refill has to keep the text for every one
// of them, while a plain string Text (e.g. [Option Info] title) must still
// follow its struct field. Both share the setTextSpriteInto path.
func TestEditorTextMapRefillKeepsText(t *testing.T) {
	oldMotif, oldBase := sys.motif, sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Select Info]\n"+
		"record.offset = 10, 20\n"+
		"[Option Info]\n"+
		"title.offset = 30, 40\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f, Sff: newSff()}
	sys.motif = *m
	m.populateDataPointers()
	m.applyPostParsePosAdjustments()

	// record: the script sets its text through start.f_getRecordText, which
	// reads record.text[gameMode()].
	record := m.SelectInfo.Record.TextSpriteData
	if record == nil {
		t.Fatal("the [Select Info] record was not populated with a TextSprite")
	}
	record.text = "RECORD 100"
	if mode, _ := editorMotifApplyClassify("Select Info", "record.offset"); mode != editorApplyRefresh {
		t.Fatalf("record.offset is classified %v, want refreshed", mode)
	}
	if err := editorApplyMotifValue(m, "Select Info", "record.offset", "50, 60", false, editorApplyRefresh); err != nil {
		t.Fatalf("saving record.offset: %v", err)
	}
	if record.text != "RECORD 100" {
		t.Errorf("record text = %q, want the script's text to survive the refill", record.text)
	}

	// [Option Info] title is a TextProperties: its Text is a Go string, so the
	// refill must re-read it rather than blank it or keep a stale runtime value.
	title := m.OptionInfo.Title.TextSpriteData
	if title == nil {
		t.Fatal("the [Option Info] title was not populated with a TextSprite")
	}
	m.OptionInfo.Title.Text = "OPTIONS"
	title.text = "stale"
	if mode, _ := editorMotifApplyClassify("Option Info", "title.offset"); mode != editorApplyRefresh {
		t.Fatalf("option title.offset is classified %v, want refreshed", mode)
	}
	if err := editorApplyMotifValue(m, "Option Info", "title.offset", "70, 80", false, editorApplyRefresh); err != nil {
		t.Fatalf("saving option title.offset: %v", err)
	}
	if title.text != "OPTIONS" {
		t.Errorf("option title text = %q, want the string field OPTIONS", title.text)
	}
}

// The position pass is global: it shifts every screen's TextSprites by their
// container offset. It has to be idempotent, or editing one screen walks the
// TextSprites of another — with the default title menu (menu.pos = 159, 158) a
// single [Select Info] title.offset save pushes the menu item texts to y=316 on
// a 320x240 screen, so the title menu comes up empty.
func TestEditorSaveDoesNotWalkOtherScreenTexts(t *testing.T) {
	oldMotif, oldBase := sys.motif, sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\n"+
		"menu.pos = 159, 158\n"+
		"[Select Info]\n"+
		"title.offset = 159, 19\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f, Sff: newSff()}
	sys.motif = *m
	// The INI is not mapped onto the struct here, so seed the field the pass reads.
	m.TitleInfo.Menu.Pos = [2]float32{159, 158}
	m.populateDataPointers()
	m.applyPostParsePosAdjustments()

	item := m.TitleInfo.Menu.Item.TextSpriteData
	if item == nil {
		t.Fatal("the title menu item was not populated with a TextSprite")
	}
	boot := item.offsetInit

	mode, _ := editorMotifApplyClassify("Select Info", "title.offset")
	if mode != editorApplyRefresh {
		t.Fatalf("title.offset is classified %v, want refreshed", mode)
	}
	if err := editorApplyMotifValue(m, "Select Info", "title.offset", "100, 50", false, mode); err != nil {
		t.Fatalf("saving title.offset: %v", err)
	}
	if item.offsetInit != boot {
		t.Errorf("saving [Select Info] title.offset walked the title menu item from %v to %v", boot, item.offsetInit)
	}
}

// menu.pos is not drawn itself: the load time position pass adds it to the
// position of the already built TextSprites. Assigning the field is therefore not
// enough, and the pass has to run again for the edit to show up.
func TestEditorApplyMotifValuePosition(t *testing.T) {
	oldMotif := sys.motif
	oldBase := sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.pos = 1000,900\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f, Sff: newSff()}
	sys.motif = *m

	if mode, _ := editorMotifApplyClassify("Title Info", "menu.pos"); mode == editorApplyReload {
		t.Fatal("menu.pos was classified reload-only, but it can be applied")
	}
	m.populateDataPointers()
	m.applyPostParsePosAdjustments()
	handle := m.TitleInfo.Menu.Item.Active.TextSpriteData
	if handle == nil {
		t.Fatal("the motif was not populated with a snapshot")
	}
	before := handle.offsetInit

	if err := editorApplyMotifValue(m, "Title Info", "menu.pos", "100,200", false, editorApplyLive); err != nil {
		t.Fatal(err)
	}
	if m.TitleInfo.Menu.Pos != [2]float32{100, 200} {
		t.Errorf("Menu.Pos = %v, want [100 200]", m.TitleInfo.Menu.Pos)
	}
	if handle.offsetInit == before {
		t.Errorf("the snapshot did not move: still %v", before)
	}
	if want := [2]float32{100, 200}; handle.offsetInit != want {
		t.Errorf("offsetInit = %v, want the new Menu.Pos %v", handle.offsetInit, want)
	}
	// The Lua script holds a handle to this object, so it must be the same one.
	if m.TitleInfo.Menu.Item.Active.TextSpriteData != handle {
		t.Error("the apply replaced the TextSprite, so the Lua script keeps drawing the old one")
	}
	// Saving the same value again must not walk the sprite further: the
	// position pass adds Menu.Pos to offsetInit, so it only stays put because
	// the refill restores the pristine offset first.
	once := handle.offsetInit
	if err := editorApplyMotifValue(m, "Title Info", "menu.pos", "100,200", false, editorApplyLive); err != nil {
		t.Fatal(err)
	}
	if handle.offsetInit != once {
		t.Errorf("a repeated save drifted the sprite: %v then %v", once, handle.offsetInit)
	}
}

// Every key of a reported [Title Info] block must apply to the running engine,
// none of them falling back to a reload. Each one is a different case: a Fade
// the script holds a handle to, a plain field the position pass consumes, a
// value only the Lua table has, and a TextSprite the menu draws.
func TestEditorApplyTitleInfoBlockLive(t *testing.T) {
	const block = "[Title Info]\n" +
		"fadein.time = 10\n" +
		"fadeout.time = 10\n" +
		"menu.pos = 20,200\n" +
		"menu.tween.factor = 0.5\n" +
		"menu.item.font = 4,0,1\n" +
		"menu.item.active.font = 4,0,1,123,206,255\n" +
		"menu.item.spacing = 0, 30\n" +
		"menu.item.layerno = 1\n" +
		"menu.item.active.layerno = 2\n"

	for _, k := range []string{
		"fadein.time", "fadeout.time", "menu.pos", "menu.tween.factor",
		"menu.item.font", "menu.item.active.font", "menu.item.spacing",
		"menu.item.layerno", "menu.item.active.layerno",
	} {
		if mode, reason := editorMotifApplyClassify("Title Info", k); mode == editorApplyReload {
			t.Errorf("%s needs a reload (%v), want it applied to the running engine", k, reason)
		}
	}

	oldMotif, oldBase := sys.motif, sys.baseDir
	sys.baseDir = t.TempDir()
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte(block))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f, Sff: newSff()}
	sys.motif = *m
	m.populateDataPointers()
	m.applyPostParsePosAdjustments()

	// The objects the Lua system script was handed at load time. Every one of
	// them is userdata wrapping this exact Go pointer, so an apply that replaced
	// the object would leave the script reading the load time values.
	fadeIn, fadeOut := m.TitleInfo.FadeIn.FadeData, m.TitleInfo.FadeOut.FadeData
	item, active := m.TitleInfo.Menu.Item.TextSpriteData, m.TitleInfo.Menu.Item.Active.TextSpriteData
	if fadeIn == nil || fadeOut == nil || item == nil || active == nil {
		t.Fatal("the motif was not populated with snapshots")
	}

	save := func(key, value string) {
		t.Helper()
		mode, _ := editorMotifApplyClassify("Title Info", key)
		if err := editorApplyMotifValue(m, "Title Info", key, value, false, mode); err != nil {
			t.Fatalf("saving %s: %v", key, err)
		}
	}

	// fadein.time / fadeout.time reach the Fade the script passes to fadeInInit.
	save("fadein.time", "30")
	save("fadeout.time", "25")
	if fadeIn.time != 30 {
		t.Errorf("FadeData.time = %v, want the saved 30", fadeIn.time)
	}
	if fadeOut.time != 25 {
		t.Errorf("FadeOut.FadeData.time = %v, want the saved 25", fadeOut.time)
	}
	// Refilled, not replaced, or main.lua's fadeInInit handle is stale.
	if m.TitleInfo.FadeIn.FadeData != fadeIn || m.TitleInfo.FadeOut.FadeData != fadeOut {
		t.Error("a fade key replaced the Fade the Lua script holds a handle to")
	}
	// A refill must not lose what the edit did not touch.
	if fadeIn.col != m.TitleInfo.FadeIn.Col {
		t.Errorf("refilling the fade changed col to %v, want %v", fadeIn.col, m.TitleInfo.FadeIn.Col)
	}

	// menu.item.layerno / menu.item.active.layerno are drawn from the TextSprite,
	// not read from the field, so the refill is what makes them take effect.
	save("menu.item.layerno", "3")
	save("menu.item.active.layerno", "4")
	if m.TitleInfo.Menu.Item.TextSpriteData != item {
		t.Fatal("a layerno key replaced the menu item TextSprite")
	}
	if item.layerno != 3 {
		t.Errorf("item.layerno = %v, want the saved 3", item.layerno)
	}
	if active.layerno != 4 {
		t.Errorf("active.layerno = %v, want the saved 4", active.layerno)
	}

	// The fonts: font[0] is the font index, font[1] the bank, and the active
	// font also carries a colour the script draws with.
	save("menu.item.font", "5,2,1")
	save("menu.item.active.font", "6,3,1,10,20,30")
	if item.bank != 2 {
		t.Errorf("item.bank = %v, want the saved 2", item.bank)
	}
	if active.bank != 3 {
		t.Errorf("active.bank = %v, want the saved 3", active.bank)
	}
	if m.TitleInfo.Menu.Item.Active.Font[3] != 10 {
		t.Errorf("active font colour = %v, want the saved 10", m.TitleInfo.Menu.Item.Active.Font[3])
	}

	// menu.pos is not drawn: the position pass adds it to the built TextSprites.
	before := item.offsetInit
	save("menu.pos", "30,300")
	if item.offsetInit == before {
		t.Error("menu.pos did not move the menu item TextSprite")
	}
	if want := [2]float32{30, 300}; item.offsetInit != want {
		t.Errorf("offsetInit = %v, want the saved Menu.Pos %v", item.offsetInit, want)
	}
	// Saving it again must land in the same place.
	once := item.offsetInit
	save("menu.pos", "30,300")
	if item.offsetInit != once {
		t.Errorf("re-saving menu.pos drifted the menu item: %v then %v", once, item.offsetInit)
	}

	// The remaining two are plain fields the Lua table is the only copy of.
	save("menu.tween.factor", "0.25")
	save("menu.item.spacing", "0, 45")
	if m.TitleInfo.Menu.Tween.Factor[0] != 0.25 {
		t.Errorf("Menu.Tween.Factor = %v, want the saved 0.25", m.TitleInfo.Menu.Tween.Factor)
	}
	if m.TitleInfo.Menu.Item.Spacing != [2]float32{0, 45} {
		t.Errorf("Menu.Item.Spacing = %v, want the saved [0 45]", m.TitleInfo.Menu.Item.Spacing)
	}
}

// Every save above also has to reach the in-memory INI, or Motif.Save would
// write the old value back over the editor's edit.
func TestEditorApplyTitleInfoBlockReachesIni(t *testing.T) {
	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.pos = 20,200\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f, Sff: newSff()}
	for key, value := range map[string]string{
		"fadein.time": "30", "fadeout.time": "25", "menu.pos": "30,300",
		"menu.tween.factor": "0.25", "menu.item.font": "5,2,1",
		"menu.item.active.font": "6,3,1,10,20,30", "menu.item.spacing": "0, 45",
		"menu.item.layerno": "3", "menu.item.active.layerno": "4",
	} {
		mode, _ := editorMotifApplyClassify("Title Info", key)
		if err := editorApplyMotifValue(m, "Title Info", key, value, false, mode); err != nil {
			t.Fatalf("saving %s: %v", key, err)
		}
	}
	for key, want := range map[string]string{
		"fadein.time": "30", "fadeout.time": "25", "menu.pos": "30,300",
		"menu.tween.factor": "0.25", "menu.item.font": "5,2,1",
		"menu.item.active.font": "6,3,1,10,20,30", "menu.item.spacing": "0, 45",
		"menu.item.layerno": "3", "menu.item.active.layerno": "4",
	} {
		if got := editorINIKeyValue(editorTestSection(t, m, "Title Info"), key); got != want {
			t.Errorf("in-memory %s = %q, want %q", key, got, want)
		}
	}
}

// A value the Lua script reads out of its own copy of the motif table is
// invisible to it until that table is updated too.
func TestEditorSyncMotifLuaTable(t *testing.T) {
	oldState, oldTable := sys.luaLState, sys.cachedMotifTable
	defer func() { sys.luaLState, sys.cachedMotifTable = oldState, oldTable }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.tween.factor = 0.5\nmenu.item.spacing = 0, 30\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Motif{IniFile: f}
	for query, value := range map[string]string{
		"title_info.menu.tween.factor": "0.25",
		"title_info.menu.item.spacing": "0, 45",
	} {
		if err := SetValueUpdate(m, m.IniFile, query, value); err != nil {
			t.Fatal(err)
		}
	}

	// The table the script is holding, built the way main.lua builds it.
	sys.luaLState = lua.NewState()
	sys.cachedMotifTable = sys.luaLState.NewTable()
	sys.cachedMotifTable.RawSetString("title_info", toLValue(sys.luaLState, m.TitleInfo))
	titleTbl, ok := sys.cachedMotifTable.RawGetString("title_info").(*lua.LTable)
	if !ok {
		t.Fatal("the motif table did not get a title_info table")
	}
	menuTbl, ok := titleTbl.RawGetString("menu").(*lua.LTable)
	if !ok {
		t.Fatal("the motif table did not get a menu table")
	}
	itemTbl, ok := menuTbl.RawGetString("item").(*lua.LTable)
	if !ok {
		t.Fatal("the motif table did not get a menu.item table")
	}
	if _, ok := menuTbl.RawGetString("tween").(*lua.LTable); !ok {
		t.Fatal("the motif table did not get a menu.tween table")
	}

	for _, query := range []string{"title_info.menu.tween.factor", "title_info.menu.item.spacing"} {
		if _, ok := editorFieldByQuery(m, query); !ok {
			t.Fatalf("the field for %s could not be resolved", query)
		}
		editorSyncMotifLuaTable(m, query)
	}

	// tween.factor is a two element array in the table (main.lua reads [1]).
	tweenTbl, ok := menuTbl.RawGetString("tween").(*lua.LTable)
	if !ok {
		t.Fatal("menu.tween is not a table")
	}
	factorTbl, ok := tweenTbl.RawGetString("factor").(*lua.LTable)
	if !ok {
		t.Fatalf("menu.tween.factor is not a table, it is %v", tweenTbl.RawGetString("factor"))
	}
	if got, ok := factorTbl.RawGet(lua.LNumber(1)).(lua.LNumber); !ok || got != 0.25 {
		t.Errorf("menu.tween.factor[1] in the Lua table = %v, want 0.25", factorTbl.RawGet(lua.LNumber(1)))
	}
	// spacing is an array, so it has to be rebuilt as a whole.
	spacingTbl, ok := itemTbl.RawGetString("spacing").(*lua.LTable)
	if !ok {
		t.Fatal("menu.item.spacing is not a table")
	}
	if got, ok := spacingTbl.RawGet(lua.LNumber(2)).(lua.LNumber); !ok || got != 45 {
		t.Errorf("menu.item.spacing[2] in the Lua table = %v, want 45", spacingTbl.RawGet(lua.LNumber(2)))
	}

	// A key the table does not have must not be invented.
	menuTbl.RawSetString("stray", lua.LNumber(1))
	before := menuTbl.RawGetString("stray")
	editorSyncMotifLuaTable(m, "title_info.menu.stray")
	if menuTbl.RawGetString("stray") != before {
		t.Error("an unknown key was written into the Lua table")
	}
}

// With no Lua state there is nothing to keep in sync, and that must be a no-op
// rather than a crash.
func TestEditorSyncMotifLuaTableNoState(t *testing.T) {
	oldState, oldTable := sys.luaLState, sys.cachedMotifTable
	sys.luaLState, sys.cachedMotifTable = nil, nil
	defer func() { sys.luaLState, sys.cachedMotifTable = oldState, oldTable }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.tween.factor = 0.5\n"))
	if err != nil {
		t.Fatal(err)
	}
	editorSyncMotifLuaTable(&Motif{IniFile: f}, "title_info.menu.tween.factor")
}

// The select screen's cell grid (start.t_grid) is built once at load, so the
// cells rows / columns / cell geometry produce only move after the grid is
// rebuilt, and so do pos and showemptyboxes: they are baked into the cached
// draw list built from that grid, and the rebuild flags that list too.
// Every other key must not trigger a rebuild.
func TestEditorSelectGridQuery(t *testing.T) {
	for _, q := range []string{
		"select_info.rows",
		"select_info.columns",
		"select_info.pos",
		"select_info.showemptyboxes",
		"select_info.cell.size",
		"select_info.cell.spacing",
		"select_info.cell.0-0.offset",
		"select_info.cell.1-2.spacing",
		"select_info.cell.*-*.skip",
	} {
		if !editorSelectGridQuery(q) {
			t.Errorf("%v does not rebuild the select grid, want a rebuild", q)
		}
	}
	for _, q := range []string{
		"select_info.cell.bg",
		"select_info.cell.random.switchtime",
		"select_info.cell.0-0.scale",
		"title_info.menu.pos",
		"select_info.rowsx",
	} {
		if editorSelectGridQuery(q) {
			t.Errorf("%v rebuilds the select grid, want no rebuild", q)
		}
	}
	// The keys the bug report is about must stay live appliers, so the grid
	// rebuild (rather than a reload) is what makes the edit show.
	for _, k := range []string{"rows", "columns", "cell.size", "cell.spacing"} {
		if mode, reason := editorMotifApplyClassify("Select Info", k); mode == editorApplyReload {
			t.Errorf("Select Info %v asks for a reload (%v), want a live apply", k, reason)
		}
	}
}

// The grid rebuild has to reach the running script: a save only shows when
// start.f_updateGrid runs and flags the cached draw list for rebuild.
func TestEditorRebuildSelectGridCallsLua(t *testing.T) {
	oldState := sys.luaLState
	defer func() { sys.luaLState = oldState }()

	sys.luaLState = lua.NewState()
	defer sys.luaLState.Close()
	called := false
	startTbl := sys.luaLState.NewTable()
	startTbl.RawSetString("f_updateGrid", sys.luaLState.NewFunction(func(l *lua.LState) int {
		called = true
		return 0
	}))
	sys.luaLState.SetGlobal("start", startTbl)

	editorRebuildSelectGrid()
	if !called {
		t.Error("saving a grid key did not run start.f_updateGrid, so the edit would never reach the screen")
	}

	// Without the script (or without the function) it stays a silent no-op.
	sys.luaLState.SetGlobal("start", lua.LNil)
	editorRebuildSelectGrid()
}

// The select title text comes from the Lua script's mode pick, not the struct,
// so a title refresh has to ask the script to re-apply it; otherwise an edited
// title.text.<mode> does not show until the player picks the mode again.
func TestEditorRebuildSelectTitleCallsLua(t *testing.T) {
	oldState := sys.luaLState
	defer func() { sys.luaLState = oldState }()

	sys.luaLState = lua.NewState()
	defer sys.luaLState.Close()
	called := false
	mainTbl := sys.luaLState.NewTable()
	mainTbl.RawSetString("f_refreshSelectTitle", sys.luaLState.NewFunction(func(l *lua.LState) int {
		called = true
		return 0
	}))
	sys.luaLState.SetGlobal("main", mainTbl)

	editorRebuildSelectTitle()
	if !called {
		t.Error("a title refresh did not run main.f_refreshSelectTitle, so the mode text would stay stale")
	}

	// Without the script (or without the function) it stays a silent no-op.
	sys.luaLState.SetGlobal("main", lua.LNil)
	editorRebuildSelectTitle()
}

// A reload swaps in a fresh motif table, so its title TextSprite starts empty:
// the reload path has to re-run the script builders, including the select
// title refresh, or the title text stays blank.
func TestEditorRebuildAfterReloadCallsLua(t *testing.T) {
	oldState := sys.luaLState
	defer func() { sys.luaLState = oldState }()

	sys.luaLState = lua.NewState()
	defer sys.luaLState.Close()
	menus, grid, title := false, false, false
	mainTbl := sys.luaLState.NewTable()
	mainTbl.RawSetString("f_rebuildMenus", sys.luaLState.NewFunction(func(l *lua.LState) int {
		menus = true
		return 0
	}))
	mainTbl.RawSetString("f_refreshSelectTitle", sys.luaLState.NewFunction(func(l *lua.LState) int {
		title = true
		return 0
	}))
	sys.luaLState.SetGlobal("main", mainTbl)
	startTbl := sys.luaLState.NewTable()
	startTbl.RawSetString("f_updateGrid", sys.luaLState.NewFunction(func(l *lua.LState) int {
		grid = true
		return 0
	}))
	sys.luaLState.SetGlobal("start", startTbl)

	editorRebuildAfterReload()
	if !menus {
		t.Error("the reload did not rebuild the motif menus")
	}
	if !grid {
		t.Error("the reload did not rebuild the select grid")
	}
	if !title {
		t.Error("the reload did not refresh the select title, so the title text would stay blank")
	}
}

func TestEditorSelectTitleQuery(t *testing.T) {
	for _, q := range []string{
		"select_info.title.offset",
		"select_info.title.font",
		"select_info.title.text.arcade",
		"select_info.title.layerno",
	} {
		if !editorSelectTitleQuery(q) {
			t.Errorf("%s is not recognized as a select title key", q)
		}
	}
	for _, q := range []string{
		"select_info.record.offset",
		"select_info.p1.name.offset",
		"title_info.title.text",
		"select_info",
	} {
		if editorSelectTitleQuery(q) {
			t.Errorf("%s was mistaken for a select title key", q)
		}
	}
}

// A save auto-reloads exactly the reload-classified keys, so this locks the
// covered set: face/portrait anims, cells, asset paths, music, background
// definitions, menu build keys and localcoord. Whatever classifies as a reload
// is reloaded by the save; live and refresh keys are untouched by that path.
func TestEditorAutoReloadCoversReloadKeys(t *testing.T) {
	for _, c := range []struct{ section, key string }{
		{"Select Info", "p1.face.scale"},
		{"Select Info", "p2.face2.random.spr"},
		{"Select Info", "stage.portrait.scale"},
		{"Select Info", "cell.bg.spr"},
		{"Files", "spr"},
		{"Music", "round1.bgm"},
		{"TitleBGdef", "time"},
		{"Title Info", "menu.itemname.arcade"},
		{"Select Info", "localcoord"},
	} {
		if mode, reason := editorMotifApplyClassify(c.section, c.key); mode != editorApplyReload {
			t.Errorf("%v %v is %v (%v), want a reload", c.section, c.key, mode, reason)
		}
	}
	// ... while these stay out of the auto-reload path.
	for _, c := range []struct{ section, key string }{
		{"Select Info", "title.offset"},
		{"Select Info", "rows"},
		{"Files", "select"},
	} {
		if mode, _ := editorMotifApplyClassify(c.section, c.key); mode == editorApplyReload {
			t.Errorf("%v %v is a reload, want it applied without one", c.section, c.key)
		}
	}
}

// Saving [Select Info] rows re-runs the position pass, which must not move the
// teammenu cursor: Anim.SetPos overwrites offsetInit, so deriving the shift
// from offsetInit added it on top of itself on every re-apply and walked the
// cursor across the screen.
func TestEditorSelectRowsSaveKeepsCursorPos(t *testing.T) {
	m := &Motif{}
	newAnimAt := func(off [2]float32) *Anim {
		// Seeded the way SetAnim seeds AnimData from the struct Offset.
		a := NewAnim(nil, "")
		a.SetPos(off[0], off[1])
		return a
	}
	seedPlayer := func(ps *PlayerSelectProperties) {
		tm := &ps.TeamMenu
		tm.SelfTitle.AnimData = newAnimAt(tm.SelfTitle.Offset)
		tm.EnemyTitle.AnimData = newAnimAt(tm.EnemyTitle.Offset)
		tm.SelfTitle.TextSpriteData, tm.EnemyTitle.TextSpriteData = &TextSprite{}, &TextSprite{}
		tm.Item.TextSpriteData, tm.Item.Active.TextSpriteData, tm.Item.Active2.TextSpriteData =
			&TextSprite{}, &TextSprite{}, &TextSprite{}
		tm.Item.Cursor.AnimData = newAnimAt(tm.Item.Cursor.Offset)
		tm.Value.Icon.AnimData = newAnimAt(tm.Value.Icon.Offset)
		tm.Value.Empty.Icon.AnimData = newAnimAt(tm.Value.Empty.Icon.Offset)
		pm := &ps.PalMenu
		pm.Bg.AnimData = newAnimAt(pm.Bg.Offset)
		pm.Number.TextSpriteData, pm.Text.TextSpriteData = &TextSprite{}, &TextSprite{}
		ps.Face.Random.AnimData = newAnimAt(ps.Face.Random.Offset)
		ps.Face.Slot.AnimData = newAnimAt(ps.Face.Slot.Offset)
		ps.Face2.Random.AnimData = newAnimAt(ps.Face2.Random.Offset)
		ps.Face2.Slot.AnimData = newAnimAt(ps.Face2.Slot.Offset)
	}
	players := []*PlayerSelectProperties{
		&m.SelectInfo.P1, &m.SelectInfo.P2, &m.SelectInfo.P3, &m.SelectInfo.P4,
		&m.SelectInfo.P5, &m.SelectInfo.P6, &m.SelectInfo.P7, &m.SelectInfo.P8,
	}
	// The reported setup: a cursor offset under a shifted menu and item.
	m.SelectInfo.Rows = 2
	ps := &m.SelectInfo.P1
	ps.TeamMenu.Pos = [2]float32{10, 20}
	ps.TeamMenu.Item.Offset = [2]float32{3, 4}
	ps.TeamMenu.Item.Cursor.Offset = [2]float32{1, 2}
	for _, p := range players {
		seedPlayer(p)
	}
	seedMenu := func(me *MenuProperties) {
		me.Arrow.Up.AnimData = newAnimAt(me.Arrow.Up.Offset)
		me.Arrow.Down.AnimData = newAnimAt(me.Arrow.Down.Offset)
		me.Item.TextSpriteData, me.Item.Selected.TextSpriteData, me.Item.Selected.Active.TextSpriteData,
			me.Item.Active.TextSpriteData, me.Item.Value.TextSpriteData, me.Item.Value.Active.TextSpriteData,
			me.Item.Value.Conflict.TextSpriteData, me.Item.Info.TextSpriteData, me.Item.Info.Active.TextSpriteData =
			&TextSprite{}, &TextSprite{}, &TextSprite{}, &TextSprite{}, &TextSprite{},
			&TextSprite{}, &TextSprite{}, &TextSprite{}, &TextSprite{}
	}
	for _, me := range []*MenuProperties{
		&m.TitleInfo.Menu, &m.OptionInfo.Menu, &m.ReplayInfo.Menu,
		&m.AttractMode.Menu, &m.OptionInfo.KeyMenu.MenuProperties,
	} {
		seedMenu(me)
	}
	m.OptionInfo.KeyMenu.P1.Playerno.TextSpriteData = &TextSprite{}
	m.OptionInfo.KeyMenu.P2.Playerno.TextSpriteData = &TextSprite{}
	m.SelectInfo.Stage.Portrait.Bg.AnimData = newAnimAt(m.SelectInfo.Stage.Portrait.Bg.Offset)
	m.SelectInfo.Stage.Portrait.Random.AnimData = newAnimAt(m.SelectInfo.Stage.Portrait.Random.Offset)
	m.SelectInfo.Stage.TextSpriteData, m.SelectInfo.Stage.Active.TextSpriteData,
		m.SelectInfo.Stage.Active2.TextSpriteData, m.SelectInfo.Stage.Done.TextSpriteData =
		&TextSprite{}, &TextSprite{}, &TextSprite{}, &TextSprite{}
	m.VsScreen.P1.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P2.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P3.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P4.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P5.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P6.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P7.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.P8.Name.TextSpriteData = &TextSprite{}
	m.VsScreen.Stage.Portrait.Bg.AnimData = newAnimAt(m.VsScreen.Stage.Portrait.Bg.Offset)
	m.VsScreen.Stage.TextSpriteData = &TextSprite{}
	m.HiscoreInfo.Item.Rank.TextSpriteData = &TextSprite{}
	m.HiscoreInfo.Item.Result.TextSpriteData = &TextSprite{}
	m.HiscoreInfo.Item.Name.TextSpriteData = &TextSprite{}

	m.applyPostParsePosAdjustments() // boot

	cur := m.SelectInfo.P1.TeamMenu.Item.Cursor.AnimData
	// Cursor.Offset + TeamMenu.Pos + Item.Offset.
	want := [2]float32{1 + 10 + 3, 2 + 20 + 4}
	if cur.x != want[0] || cur.y != want[1] {
		t.Fatalf("cursor pos = [%v %v], want %v", cur.x, cur.y, want)
	}

	// Saving rows assigns the field and re-runs the same pass.
	if err := SetValue(m, "select_info.rows", "3"); err != nil {
		t.Fatal(err)
	}
	if m.SelectInfo.Rows != 3 {
		t.Fatalf("Rows = %v, want the saved 3", m.SelectInfo.Rows)
	}
	m.applyPostParsePosAdjustments()
	if cur.x != want[0] || cur.y != want[1] {
		t.Errorf("saving rows moved the cursor to [%v %v], want %v", cur.x, cur.y, want)
	}

	// And again: the pass must be idempotent no matter how often it re-runs.
	m.applyPostParsePosAdjustments()
	if cur.x != want[0] || cur.y != want[1] {
		t.Errorf("re-running the pass moved the cursor to [%v %v], want %v", cur.x, cur.y, want)
	}
}

// The motif menu tables are built once at load; the reload re-runs those
// builders (main.f_rebuildMenus), so the keys they consume ask for a reload
// instead of reporting a live apply the menu tables would not reflect.
func TestEditorMotifMenuBuildKey(t *testing.T) {
	for _, q := range []string{
		"attract_mode.enabled",
		"title_info.menu.itemname.arcade",
		"option_info.menu.itemname.exit",
		"option_info.keymenu.itemname.rumble",
		"attract_mode.menu.itemname.exit",
		"title_info.title.text",
		"option_info.title.text",
		"title_info.menu.title.uppercase",
	} {
		if !editorMotifMenuBuildKey(q) {
			t.Errorf("%v is not treated as a menu build key", q)
		}
	}
	for _, q := range []string{
		"title_info.menu.itemname_order",
		"title_info.menu.pos",
		"title_info.menu.item.spacing",
		"select_info.rows",
		"replay_info.title.text",
	} {
		if editorMotifMenuBuildKey(q) {
			t.Errorf("%v is treated as a menu build key, want a live apply", q)
		}
	}
	// The keys the reload now rebuilds must be classified as reloads.
	for _, c := range []struct{ section, key string }{
		{"Attract Mode", "enabled"},
		{"Title Info", "menu.itemname.arcade"},
		{"Option Info", "keymenu.itemname.rumble"},
	} {
		if mode, reason := editorMotifApplyClassify(c.section, c.key); mode != editorApplyReload {
			t.Errorf("%v.%v is %v (%v), want a reload", c.section, c.key, mode, reason)
		}
	}
}

// The apply is posted to the engine thread and its result is waited for, so the
// game loop never sees a half written motif.
func TestEditorMotifKeyCoverage(t *testing.T) {
	// How much of a real motif is actually live-appliable? A key inside a
	// struct that PopulateDataPointers snapshots (TextProperties, and friends)
	// can be assigned but the screen keeps drawing the load time copy.
	secs := editorMotifStructure()
	if len(secs) == 0 {
		t.Skip("no motif structure")
	}
	var live, refresh, reload int
	for _, sec := range secs {
		for _, k := range sec.Keys {
			if k.Dynamic {
				continue
			}
			switch mode, _ := editorMotifApplyClassify(sec.Section, k.Key); mode {
			case editorApplyLive:
				live++
			case editorApplyRefresh:
				refresh++
			default:
				reload++
			}
		}
	}
	t.Logf("motif keys: %d assigned directly, %d assigned and refreshed, %d need a reload",
		live, refresh, reload)
	if live+refresh == 0 {
		t.Error("no motif key is applicable at all; the classification is too strict")
	}
	if reload == 0 {
		t.Error("no motif key needs a reload; the classification is too loose")
	}
	if refresh == 0 {
		t.Error("no motif key is refreshable; the snapshots would never rebuild")
	}
	// The reported case must be applied, through a refresh.
	if mode, _ := editorMotifApplyClassify("Title Info", "menu.item.active.font"); mode != editorApplyRefresh {
		t.Error("menu.item.active.font is not refreshed, so the menu would keep drawing the old font")
	}
}

func TestEditorRunOnMainThread(t *testing.T) {
	// Stand in for System.await, which is what drains this queue in the game.
	stop := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case f := <-sys.mainThreadTask:
				f()
			case <-stop:
				return
			}
		}
	}()
	defer func() {
		close(stop)
		<-drained
	}()

	if err := editorRunOnMainThread(func() error { return nil }); err != nil {
		t.Errorf("a successful task reported %v", err)
	}
	want := fmt.Errorf("boom")
	if err := editorRunOnMainThread(func() error { return want }); err != want {
		t.Errorf("the task error was %v, want %v", err, want)
	}
	// A nil motif is reported, not swallowed.
	if err := editorRunOnMainThread(func() error { return editorApplyMotifValue(nil, "a", "b", "c", false, editorApplyLive) }); err == nil {
		t.Error("a nil motif was accepted through the dispatcher")
	}
}

// The reported scenario: editing menu.item.active.font in the configured motif.
// The whole apply, classification included, has to reach the engine thread, so
// the mode that is reported is the one the engine thread decided, and the live
// struct is only touched there.
func TestEditorApplyMotifSyncOnEngineThread(t *testing.T) {
	oldMotif, oldBase := sys.motif, sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.motif, sys.baseDir = oldMotif, oldBase }()

	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.item.active.font = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	sys.motif = Motif{IniFile: f, Sff: newSff()}

	// Stand in for System.await, which drains this queue on the engine thread.
	stop := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case f := <-sys.mainThreadTask:
				f()
			case <-stop:
				return
			}
		}
	}()
	defer func() {
		close(stop)
		<-drained
	}()

	applied, _, reason, warning, err := editorApplyMotifSync("Title Info", "menu.item.active.font", "2", false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied {
		t.Errorf("applied = false (reason %q), want the key to be refreshed live", reason)
	}
	if got := sys.motif.TitleInfo.Menu.Item.Active.Font[0]; got != 2 {
		t.Errorf("Font[0] = %v, want the assigned 2", got)
	}
	if got := editorINIKeyValue(editorTestSection(t, &sys.motif, "Title Info"), "menu.item.active.font"); got != "2" {
		t.Errorf("in-memory font = %q, want 2", got)
	}
	// The fixture loads no fonts, so index 2 resolves to nothing: the struct
	// is assigned but the snapshot keeps its font, and the save says so
	// instead of a bare "applied".
	if warning == "" {
		t.Error("no warning for an unloaded font index, so the toast would claim a full apply")
	} else if !strings.Contains(warning, "font 2 is not loaded") {
		t.Errorf("warning = %q, want it to name the unloaded font", warning)
	}
}

func TestEditorFontResolutionWarning(t *testing.T) {
	oldMotif := sys.motif
	defer func() { sys.motif = oldMotif }()
	f, err := ini.LoadSources(editorINIOptions(), []byte("[Title Info]\nmenu.item.active.font = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	sys.motif = Motif{IniFile: f, Sff: newSff(), Fnt: map[int]*Fnt{1: {}, 2: {}}}
	query := editorMotifQuery("Title Info", "menu.item.active.font")
	// Populate the struct the way a save would: the INI text alone leaves
	// Font at its zero value, and the warning reads the struct field.
	if err := SetValueUpdate(&sys.motif, sys.motif.IniFile, query, "1, 0, 0, 255, 255, 255, 255, -1"); err != nil {
		t.Fatal(err)
	}

	// A loaded index resolves quietly.
	if w := editorFontResolutionWarning(&sys.motif, query); w != "" {
		t.Errorf("a loaded font warns: %q", w)
	}
	// Anything else with a font leaf stays quiet too: non-font keys, the -1
	// default (not a font), and values that are not a font array.
	for _, c := range []struct{ section, key, value string }{
		{"Title Info", "menu.item.spacing", "10, 10"},
		{"Title Info", "menu.item.active.font", "-1, 0, 0, 255, 255, 255, 255, -1"},
	} {
		// Assign only: the fixture has no populated snapshots, so there is no
		// screen to reapply.
		if err := SetValueUpdate(&sys.motif, sys.motif.IniFile, editorMotifQuery(c.section, c.key), c.value); err != nil {
			t.Fatal(err)
		}
		if w := editorFontResolutionWarning(&sys.motif, editorMotifQuery(c.section, c.key)); w != "" {
			t.Errorf("%s warns: %q", c.key, w)
		}
	}
	// An index with no loaded font warns.
	if err := SetValueUpdate(&sys.motif, sys.motif.IniFile, query, "9, 0, 0, 255, 255, 255, 255, -1"); err != nil {
		t.Fatal(err)
	}
	if w := editorFontResolutionWarning(&sys.motif, query); !strings.Contains(w, "font 9 is not loaded") {
		t.Errorf("an unloaded font warns %q, want it to name font 9", w)
	}
}

func editorTestSection(t *testing.T, m *Motif, name string) *ini.Section {
	t.Helper()
	sec, err := m.IniFile.GetSection(name)
	if err != nil {
		t.Fatalf("section %v: %v", name, err)
	}
	return sec
}

// -------------------------------------------------------------------
// Sprite preview (/api/sff)
// -------------------------------------------------------------------

func TestEditorSffIndex(t *testing.T) {
	if v, err := editorSffIndex("9000", "group"); err != nil || v != 9000 {
		t.Errorf("editorSffIndex(9000) = %v, %v; want 9000, nil", v, err)
	}
	if v, err := editorSffIndex(" 0 ", "number"); err != nil || v != 0 {
		t.Errorf("editorSffIndex(\" 0 \") = %v, %v; want 0, nil", v, err)
	}
	for _, s := range []string{"", "abc", "-1", "70000", "1.5"} {
		if _, err := editorSffIndex(s, "group"); err == nil {
			t.Errorf("editorSffIndex(%q) was accepted, want an error", s)
		}
	}
}

// The pixel data is what the browser shows, so check the palette unpacking.
func TestEditorSffSpriteImage(t *testing.T) {
	sff := newSff()

	// 8 bit: one palette index per pixel, colors packed 0xAABBGGRR.
	spr := &Sprite{Size: [2]uint16{2, 1}, pendingW: 2, pendingH: 1, pendingDepth: 8}
	spr.pendingData = []byte{0, 1}
	spr.Pal = []uint32{0, 0xff00ff00}
	img := editorSffSpriteImage(sff, spr)
	if img == nil {
		t.Fatal("paletted sprite produced no image")
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 1 {
		t.Errorf("image is %v, want 2x1", img.Bounds())
	}
	if got := img.RGBAAt(0, 0); got != (stdcolor.RGBA{0, 0, 0, 0}) {
		t.Errorf("index 0 = %v, want fully transparent", got)
	}
	if got := img.RGBAAt(1, 0); got != (stdcolor.RGBA{0, 255, 0, 255}) {
		t.Errorf("index 1 = %v, want opaque green", got)
	}

	// 24 bit: RGB in, opaque out.
	spr = &Sprite{pendingW: 1, pendingH: 1, pendingDepth: 24}
	spr.pendingData = []byte{10, 20, 30}
	if img = editorSffSpriteImage(sff, spr); img == nil {
		t.Fatal("24 bit sprite produced no image")
	} else if got := img.RGBAAt(0, 0); got != (stdcolor.RGBA{10, 20, 30, 255}) {
		t.Errorf("24 bit pixel = %v, want {10 20 30 255}", got)
	}

	// 32 bit: RGBA passes through untouched.
	spr = &Sprite{pendingW: 1, pendingH: 1, pendingDepth: 32}
	spr.pendingData = []byte{10, 20, 30, 40}
	if img = editorSffSpriteImage(sff, spr); img == nil {
		t.Fatal("32 bit sprite produced no image")
	} else if got := img.RGBAAt(0, 0); got != (stdcolor.RGBA{10, 20, 30, 40}) {
		t.Errorf("32 bit pixel = %v, want {10 20 30 40}", got)
	}
}

// Nothing readable.
func TestEditorSffSpriteImageUnreadable(t *testing.T) {
	sff := newSff()
	for name, bad := range map[string]*Sprite{
		"nil sprite":    nil,
		"no pixels":     {pendingW: 1, pendingH: 1, pendingDepth: 8},
		"zero size":     {pendingDepth: 8, pendingData: []byte{0}},
		"short buffer":  {pendingW: 4, pendingH: 4, pendingDepth: 8, pendingData: []byte{0, 1, 2}},
		"unknown depth": {pendingW: 1, pendingH: 1, pendingDepth: 16, pendingData: []byte{0, 0}},
		"no palette":    {pendingW: 1, pendingH: 1, pendingDepth: 8, pendingData: []byte{0}},
	} {
		if img := editorSffSpriteImage(sff, bad); img != nil {
			t.Errorf("%s produced an image, want nil", name)
		}
	}
}

func TestEditorHandleSFFErrors(t *testing.T) {
	oldBase := sys.baseDir
	dir := t.TempDir()
	sys.baseDir = dir
	defer func() { sys.baseDir = oldBase }()

	cases := []struct {
		name string
		url  string
		want int
	}{
		{"no file", "/api/sff?group=9000&number=0", http.StatusBadRequest},
		{"no group", "/api/sff?file=a.sff&number=0", http.StatusBadRequest},
		{"no number", "/api/sff?file=a.sff&group=9000", http.StatusBadRequest},
		{"bad group", "/api/sff?file=a.sff&group=x&number=0", http.StatusBadRequest},
		{"outside the game folder", "/api/sff?file=../a.sff&group=1&number=0", http.StatusBadRequest},
		{"not a sff", "/api/sff?file=a.def&group=1&number=0", http.StatusBadRequest},
		{"missing file", "/api/sff?file=nope.sff&group=1&number=0", http.StatusNotFound},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		editorHandleSFF(rr, httptest.NewRequest(http.MethodGet, c.url, nil))
		if rr.Code != c.want {
			t.Errorf("%s: GET %v = %d, want %d (%s)", c.name, c.url, rr.Code, c.want, rr.Body.String())
		}
	}
	// A .sff that is really a .def is reported as unreadable, not served.
	if err := os.WriteFile(filepath.Join(dir, "a.sff"), []byte("[Info]\nname = x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	editorHandleSFF(rr, httptest.NewRequest(http.MethodGet, "/api/sff?file=a.sff&group=1&number=0", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("GET /api/sff on a broken .sff = %d, want 500", rr.Code)
	}
}

// End to end against a real .sff from the local deploy folder.
func TestEditorHandleSFFServesPNG(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "deploy"))
	if err != nil {
		t.Fatal(err)
	}
	rel := "chars/kfm/kfm.sff"
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Skip("no local kfm.sff")
	}
	oldwd, _ := os.Getwd()
	oldBase := sys.baseDir
	defer func() {
		_ = os.Chdir(oldwd)
		sys.baseDir = oldBase
		editorSffCache.mu.Lock()
		editorSffCache.m = map[string]*Sff{}
		editorSffCache.mu.Unlock()
	}()
	if err := os.Chdir(root); err != nil {
		t.Skip("cannot enter the deploy folder")
	}
	sys.baseDir = "./"

	// Find a sprite the file actually has, so the test does not depend on one
	// particular sprite number.
	sff, err := editorLoadSff(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Skipf("unable to read %v: %v", rel, err)
	}
	var served bool
	for key, spr := range sff.sprites {
		if editorSffSpriteImage(sff, spr) == nil {
			continue
		}
		rr := httptest.NewRecorder()
		url := "/api/sff?file=" + rel + "&group=" + strconv.Itoa(int(key[0])) + "&number=" + strconv.Itoa(int(key[1]))
		editorHandleSFF(rr, httptest.NewRequest(http.MethodGet, url, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %v = %d (%s)", url, rr.Code, rr.Body.String())
		}
		if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("Content-Type = %q, want image/png", ct)
		}
		img, err := png.Decode(bytes.NewReader(rr.Body.Bytes()))
		if err != nil {
			t.Fatalf("GET %v did not return a PNG: %v", url, err)
		}
		if img.Bounds().Dx() != int(spr.Size[0]) || img.Bounds().Dy() != int(spr.Size[1]) {
			t.Errorf("sprite %v,%v is %v, want %vx%v", key[0], key[1], img.Bounds(), spr.Size[0], spr.Size[1])
		}
		served = true
		break
	}
	if !served {
		t.Skipf("no drawable sprite in %v", rel)
	}

	// An unknown sprite is a 404, not a blank image.
	rr := httptest.NewRecorder()
	editorHandleSFF(rr, httptest.NewRequest(http.MethodGet, "/api/sff?file="+rel+"&group=65000&number=65000", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /api/sff for a missing sprite = %d, want 404", rr.Code)
	}

	// The decoded file is cached, so a second request does not re-read it.
	if s2, err := editorLoadSff(filepath.Join(root, filepath.FromSlash(rel))); err != nil || s2 != sff {
		t.Error("editorLoadSff did not return the cached Sff")
	}
}

// -------------------------------------------------------------------
// End to end against the local deploy folder (skipped when absent)
// -------------------------------------------------------------------

func TestEditorAgainstConfiguredMotif(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "deploy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "save", "config.ini")); err != nil {
		t.Skip("no local deploy folder")
	}
	oldwd, _ := os.Getwd()
	oldBase := sys.baseDir
	oldFlags := sys.cmdFlags
	defer func() {
		_ = os.Chdir(oldwd)
		sys.baseDir = oldBase
		sys.cmdFlags = oldFlags
	}()
	if err := os.Chdir(root); err != nil {
		t.Skip("cannot enter the deploy folder")
	}
	sys.baseDir = "./"
	sys.cmdFlags = map[string]string{"-config": "save/config.ini"}

	if editorMotifPath() == "" {
		t.Fatal("editorMotifPath() found no motif")
	}
	if editorSelectDefPath() == "" {
		t.Error("editorSelectDefPath() found no select.def")
	}

	sections := editorMotifSections()
	if len(sections) < 10 {
		t.Fatalf("expected several motif sections, got %d", len(sections))
	}
	var title *editorMotifSectionJSON
	for i := range sections {
		if sections[i].Title == "title_info" {
			title = &sections[i]
		}
	}
	if title == nil {
		t.Fatal("[Title Info] is missing from the configured motif breakdown")
	}
	if title.Defined == 0 {
		t.Error("[Title Info] has no defined keys")
	}
	// The Editor submenu has to be declared by the motif for the menu to show it.
	editorKeys := map[string]string{}
	for _, k := range title.Keys {
		editorKeys[strings.ToLower(k.Key)] = k.Value
	}
	for _, want := range []string{
		"menu.itemname.editor",
		"menu.itemname.editor.editormotif",
		"menu.itemname.editor.editorstage",
		"menu.itemname.editor.editorcharacter",
		"menu.itemname.editor.back",
	} {
		if editorKeys[want] == "" {
			t.Errorf("the configured motif does not declare %q", want)
		}
	}
}
