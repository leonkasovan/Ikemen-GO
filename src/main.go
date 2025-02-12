package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	_ "embed" // Support for go:embed resources
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	lua "github.com/ikemen-engine/Ikemen-GO/packages/gopher-lua"
	"github.com/ikemen-engine/Ikemen-GO/packages/physfs"
)

var Version = "eXtra"
var BuildTime = "20250201"

//go:embed assets.zip
var assetsZip []byte

//go:embed screenpack.zip
var screenpackZip []byte

func renameFilesToLowerCase(root string) error {
	// Walk through the directory recursively
	physfs.Walk(root, func(path string, isDir bool) error {
		// Skip directories
		if isDir {
			return nil
		}

		// Get the directory and the lowercase file name
		fileName := path
		ext := filepath.Ext(fileName)
		fileNameWithoutExt := strings.TrimSuffix(fileName, ext)
		lowercaseName := fileNameWithoutExt + strings.ToLower(ext)

		// Check if renaming is needed
		if path != lowercaseName {
			// Rename the file
			err := os.Rename(path, lowercaseName)
			if err != nil {
				return fmt.Errorf("failed to rename %s to %s: %w", path, lowercaseName, err)
			}
			fmt.Printf("Renamed: %s -> %s\n", path, lowercaseName)
		}
		return nil
	})
	return nil
}

// extractFileFromEmbed extracts a specific file from the embedded ZIP content by its name  into current dir.
func extractFileFromEmbed(content []byte, filename string) error {
	zipReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return err
	}

	// Search for the file in the archive
	for _, file := range zipReader.File {
		if file.Name == filename {
			// Ensure the directory exists before creating the file
			if err := os.MkdirAll(filepath.Dir(file.Name), os.ModePerm); err != nil {
				return err
			}

			fileReader, err := file.Open()
			if err != nil {
				return err
			}
			defer fileReader.Close()

			outFile, err := os.Create(file.Name)
			if err != nil {
				return err
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, fileReader)
			if err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("file %s not found in archive", filename)
}

// extractEmbed extracts all files from the embedded ZIP content into current dir.
func extractEmbed(content []byte) error {
	// Open the embedded zip file from the byte slice
	zipReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return err
	}

	// Iterate over the files in the zip archive
	for _, file := range zipReader.File {
		// fmt.Printf("Extracting: %s\n", file.Name)

		// Open the file inside the zip archive
		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		// Handle directories by creating them first
		if file.FileInfo().IsDir() {
			err := os.MkdirAll(file.Name, os.ModePerm)
			if err != nil {
				return err
			}
			continue
		}

		// Ensure the directory exists before creating the file
		if err := os.MkdirAll(filepath.Dir(file.Name), os.ModePerm); err != nil {
			return err
		}

		// Create the destination file on disk
		outFile := physfs.OpenWrite(file.Name)
		if outFile == nil {
			return fmt.Errorf("can not write %f", file.Name)
		}
		defer outFile.Close()

		// Copy the file contents to the destination file
		_, err = io.Copy(outFile, fileReader)
		if err != nil {
			return err
		}
	}
	return nil
}

// extractFile extracts a file from the ZIP archive to the specified path
func extractFile(f *zip.File, filePath string) error {
	// Open the file inside the ZIP archive
	srcFile, err := f.Open()
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create the destination file
	destFile := physfs.OpenWrite(filePath)
	if destFile == nil {
		return fmt.Errorf("can not write %s", filePath)
	}
	defer destFile.Close()

	// Copy the file content
	_, err = io.Copy(destFile, srcFile)
	return err
}

func stringInSlice(target string, slice []string) bool {
	for _, str := range slice {
		if str == target {
			return true
		}
	}
	return false
}

// Update Section [Characters] in select.def based on [char] directory
func updateCharInSelectDef(fname string) error {
	// Open the file
	filename := fname
	fmt.Printf("[main.go] fname=%v filename=%v\n", fname, filename)
	file := physfs.OpenRead(filename)
	if file == nil {
		return fmt.Errorf("Error: can't open.read file %v\n", filename)
	}

	// Open or create the file
	file2 := physfs.OpenWrite(filename + ".update")
	if file2 == nil {
		file.Close()
		return fmt.Errorf("Error: can't open.write file %v", filename+".update")
	}

	// Create a buffered writer
	writer := bufio.NewWriter(file2)

	// Create a new scanner
	scanner := bufio.NewScanner(file)

	// Loop through each line
	var result []string
	var line string
	chars := make([]string, 0, 20)
	section := 0
	for scanner.Scan() {
		line = strings.ToLower(scanner.Text())
		if len(line) < 1 {
			continue
		}
		if line[0] == ';' { // skip comment
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if len(line) < 2 {
			continue
		}
		if line[0] == ' ' && line[1] == ';' { // skip nested comment
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if strings.Contains(line, "[characters]") {
			section = 1
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if strings.Contains(line, "[extrastages]") {
			// Open the directory
			files, err := os.ReadDir("chars")
			if err != nil {
				file.Close()
				file2.Close()
				return err
			}

			// List only directories
			for _, file := range files {
				if file.IsDir() {
					if !stringInSlice(file.Name(), chars) {
						fmt.Printf(" add new char: %v\n", file.Name())
						writer.WriteString(file.Name() + ", random\n")
					}
				}
			}
			section = 2
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if section == 1 {
			result = regexp.MustCompile(`^(.+?),`).FindStringSubmatch(scanner.Text())
			if result != nil {
				writer.WriteString(scanner.Text() + "\n")
				chars = append(chars, result[1])
				fmt.Printf(" existing char: %v\n", result[1])
				continue
			}
		}
		writer.WriteString(scanner.Text() + "\n")
	}
	writer.Flush()
	file.Close()
	file2.Close()
	var err error
	err = os.Rename(filename, filename+".bak")
	if err != nil {
		fmt.Printf("[main.go] '%v' => '%v'\n\terr: %v\n", filename, filename+".bak", err)
		return err
	}
	err = os.Rename(filename+".update", filename)
	if err != nil {
		fmt.Printf("[main.go] '%v' => '%v'\n\terr: %v\n", filename+".update", filename, err)
		return err
	}

	return scanner.Err()
}

// Update Section [ExtraStages] in select.def based on files *.def in [stages] directory
func updateStageInSelectDef(fname string) error {
	path_sep1 := ""
	path_sep2 := ""

	// Open the file
	filename := fname
	file := physfs.OpenRead(filename)
	if file == nil {
		return fmt.Errorf("Error: can't open.read file %v", filename)
	}

	// Open or create the file
	file2 := physfs.OpenWrite(filename + ".update")
	if file2 == nil {
		file.Close()
		return fmt.Errorf("Error: can't open.write file %v", filename)
	}

	// Create a buffered writer
	writer := bufio.NewWriter(file2)

	// Create a new scanner
	scanner := bufio.NewScanner(file)

	// Loop through each line
	var line string
	stages := make([]string, 0, 20)
	section := 0
	for scanner.Scan() {
		line = strings.ToLower(scanner.Text())
		if len(line) < 1 {
			continue
		}
		if line[0] == ';' { // skip comment
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if len(line) < 2 {
			continue
		}
		if line[0] == ' ' && line[1] == ';' { // skip nested comment
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if strings.Contains(line, "[characters]") {
			section = 1
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if strings.Contains(line, "[extrastages]") {
			section = 2
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if strings.Contains(line, "[options]") {
			// Combine directory and pattern
			searchPattern := filepath.Join("stages", "*.def")

			// Get the list of files matching the pattern
			files, err := filepath.Glob(searchPattern)
			if err != nil {
				file.Close()
				file2.Close()
				return err
			}

			// Print the matching files
			for _, file := range files {
				file = strings.Replace(file, path_sep2, path_sep1, -1)
				if !stringInSlice(file, stages) {
					fmt.Printf(" add new stage: %v\n", file)
					writer.WriteString(file + "\n")
				}
			}
			section = 3
			writer.WriteString(scanner.Text() + "\n")
			continue
		}
		if section == 2 {
			writer.WriteString(scanner.Text() + "\n")
			stages = append(stages, scanner.Text())
			fmt.Printf(" existing stage: %v\n", scanner.Text())
			if path_sep1 == "" {
				if strings.Contains(scanner.Text(), "/") {
					path_sep1 = "/"
					path_sep2 = "\\"
				}
				if strings.Contains(scanner.Text(), "\\") {
					path_sep1 = "\\"
					path_sep2 = "/"
				}
			}
			continue
		}
		writer.WriteString(scanner.Text() + "\n")
	}
	writer.Flush()
	file.Close()
	file2.Close()
	os.Rename(filename, filename+".bak")
	os.Rename(filename+".update", filename)
	return scanner.Err()
}

// upgrade config.json from older version (below 0.98.x)
func init() {
	runtime.LockOSThread()
}

// Checks if error is not null, if there is an error it displays a error dialogue box and crashes the program.
func chk(err error) {
	if err != nil {
		ShowErrorDialog(err.Error())
		panic(err)
	}
}

// Extended version of 'chk()'
func chkEX(err error, txt string, crash bool) bool {
	if err != nil {
		ShowErrorDialog(txt + err.Error())
		if crash {
			panic(Error(txt + err.Error()))
		}
		return true
	}
	return false
}

func createLog(p string) *physfs.File {
	f := physfs.OpenWrite(p)
	if f == nil {
		fmt.Printf("Error: open log file %v\n", p)
		os.Exit(-1)
	}
	return f
}

func main() {
	fmt.Printf("\nIkemen GO! %v %v\n", Version, BuildTime)
	if !physfs.Init(os.Args[0]) {
		fmt.Println("Error: initialize file system")
		return
	}
	defer physfs.Deinit()

	// Load Order:
	// 1. *.z0
	// 2. current directory "."
	// 3. *.zip
	
	files, err := os.ReadDir(".")
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	// Find zp0 files and mount it
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".zp0") {
			// Open the file
			if !physfs.Mount(file.Name(), "/", 1) {
				fmt.Printf("Mounting %v [FAIL]\n", file.Name())
			} else {
				fmt.Printf("Mounting %v [OK]\n", file.Name())
			}
		}
	}

	// Mount the current directory
	currentDir, _ := os.Getwd()
	if !physfs.Mount(currentDir, "/", 1) {
		fmt.Printf("Mounting directory \"%v\" [FAIL]\n", currentDir)
	} else {
		fmt.Printf("Mounting directory \"%v\" [OK]\n", currentDir)
	}

	// Find zip files and mount it
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".zip") {
			// Open the file
			if !physfs.Mount(file.Name(), "/", 1) {
				fmt.Printf("Mounting %v [FAIL]\n", file.Name())
			} else {
				fmt.Printf("Mounting %v [OK]\n", file.Name())
			}
		}
	}

	// Set Write Directory
	physfs.SetWriteDir(currentDir)

	is_mugen_game := false
	fmt.Printf("Ikemen running on OS=[%v] ARCH=[%v]\n", runtime.GOOS, runtime.GOARCH)

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Error getting executable path:", err)
	} else {
		// Change the context for Darwin if we're in an app bundle
		if isRunningInsideAppBundle(exePath) {
			os.Chdir(path.Dir(exePath))
			os.Chdir("../../../")
		}
	}

	// Check if the "external" directory exists and data/mugen.cfg, if not exists then extract assets from embedded
	if !physfs.Exists("external") && physfs.Exists("data/mugen.cfg") {
		err := extractEmbed(assetsZip)
		if err != nil {
			fmt.Printf("[main.go] Error extracting asset: %v\n", err)
		}
		fmt.Println("[main.go] Mugen Game detected. Assets extraction completed successfully.")
		is_mugen_game = true
	}

	processCommandLine()
	if _, ok := sys.cmdFlags["-game"]; ok {
		dir := filepath.Dir(sys.cmdFlags["-game"])
		base := filepath.Base(sys.cmdFlags["-game"])
		name := base[:len(base)-len(filepath.Ext(base))] // Remove the extension from the base name

		err := os.Chdir(filepath.Join(dir, name))
		if err != nil {
			fmt.Println("Error changing directory:", err)
			panic(err)
		}
	}
	if _, ok := sys.cmdFlags["-updatechar"]; ok {
		fmt.Printf("[main.go] Update data/select.def based on [char] directory\n")
		err := updateCharInSelectDef("data/select.def")
		if err != nil {
			fmt.Printf("[main.go] %v\n", err)
		}
	}

	if _, ok := sys.cmdFlags["-updatestage"]; ok {
		fmt.Printf("[main.go] Update data/select.def based on [stages] directory\n")
		err := updateStageInSelectDef("data/select.def")
		if err != nil {
			fmt.Printf("[main.go] %v\n", err)
		}
	}

	if _, ok := sys.cmdFlags["-installrun"]; ok {
		fmt.Printf("[main.go] Install default screenpack\n")
		err := extractEmbed(screenpackZip)
		if err != nil {
			fmt.Printf("[main.go] Error extracting screenpack: %v\n", err)
		}
		err = extractEmbed(assetsZip)
		if err != nil {
			fmt.Printf("[main.go] Error extracting asset: %v\n", err)
		}
	}

	if _, ok := sys.cmdFlags["-install"]; ok {
		fmt.Printf("[main.go] Install default screenpack\n")
		err := extractEmbed(screenpackZip)
		if err != nil {
			fmt.Printf("[main.go] Error extracting screenpack: %v\n", err)
		}
		err = extractEmbed(assetsZip)
		if err != nil {
			fmt.Printf("[main.go] Error extracting asset: %v\n", err)
		}
		os.Exit(0)
	}

	// Make save directories, if they don't exist
	os.Mkdir("save", os.ModeSticky|0755)
	os.Mkdir("save/replays", os.ModeSticky|0755)

	// Config file path
	cfgPath := "save/config.ini"
	// If a different config file is defined in the command line parameters, use it instead
	if _, ok := sys.cmdFlags["-config"]; ok {
		cfgPath = sys.cmdFlags["-config"]
	}

	if cfg, err := loadConfig(cfgPath, is_mugen_game); err != nil {
		chk(err)
	} else {
		sys.cfg = *cfg
	}

	if _, ok := sys.cmdFlags["-validate"]; ok {
		if !physfs.FileExist("external/script/audit.lua") {
			err := extractFileFromEmbed(assetsZip, "external/script/audit.lua")
			if err != nil {
				fmt.Printf("[main.go] Error extracting audit.lua: %v\n", err)
				os.Exit(0)
			}
		}
		if !physfs.FileExist("external/script/json.lua") {
			err := extractFileFromEmbed(assetsZip, "external/script/json.lua")
			if err != nil {
				fmt.Printf("[main.go] Error extracting json.lua: %v\n", err)
				os.Exit(0)
			}
		}
		renameFilesToLowerCase("chars")
		renameFilesToLowerCase("stages")
		renameFilesToLowerCase("font")
		renameFilesToLowerCase("data")
		renameFilesToLowerCase("sound")
		l := lua.NewState()
		l.Options.IncludeGoStackTrace = true
		l.OpenLibs()
		systemScriptInit(l)
		fmt.Printf("\n\n==================================\nValidating included the game assets...\n")
		if err := l.DoFile("external/script/audit.lua"); err != nil {
			fmt.Printf("[main.go] Error running validation script: %v\n", err)
		}
		os.Exit(0)
	}

	if _, ok := sys.cmdFlags["-fix"]; ok {
		renameFilesToLowerCase("chars")
		renameFilesToLowerCase("stages")
		renameFilesToLowerCase("font")
		renameFilesToLowerCase("data")
		renameFilesToLowerCase("sound")
		l := lua.NewState()
		l.Options.IncludeGoStackTrace = true
		l.OpenLibs()
		systemScriptInit(l)
		fmt.Printf("\n\n==================================\nFixing game assets...\n")
		if err := l.DoFile("external/script/fix.lua"); err != nil {
			fmt.Printf("[main.go] Error running fix script: %v\n", err)
		}
		os.Exit(0)
	}

	// Check if the main lua file exists.
	if !physfs.Exists(sys.cfg.Config.System) {
		fmt.Printf("Error: script %v NOT found.\n", sys.cfg.Config.System)
		os.Exit(-1)
	}

	// Initialize game and create window
	sys.luaLState = sys.init(sys.gameWidth, sys.gameHeight)
	defer sys.shutdown()

	// Begin processing game using its lua scripts
	no := 1
	lua_script, err := LoadText(sys.cfg.Config.System)
	if err != nil {
		fmt.Printf("[main.go]physfs LastError: %v\n", physfs.GetError())
		fmt.Printf("[main.go]Error: %v\n", err)
		return
	}
	if err := sys.luaLState.DoString(lua_script); err != nil {
		fmt.Printf("[%v]Error: %v\n", err, no)
		// Display error logs.
		errorLog := createLog("Ikemen.log")
		fmt.Fprintln(errorLog, err)
		switch err.(type) {
		case *lua.ApiError:
			errstr := strings.Split(err.Error(), "\n")[0]
			if len(errstr) < 10 || errstr[len(errstr)-10:] != "<game end>" {
				ShowErrorDialog(fmt.Sprintf("%s\n\nError saved to Ikemen.log", err))
				panic(err)
			}
		default:
			ShowErrorDialog(fmt.Sprintf("%s\n\nError saved to Ikemen.log", err))
			panic(err)
		}
		errorLog.Close()
		no += 1
	}

	// Mute and close BGM
	sys.bgm.Open("", 1, 100, 0, 0, 0, 1.0, 1)

	// Unmount current directory
	if !physfs.Unmount(currentDir) {
		fmt.Printf("Unmounting directory \"%v\" [FAIL]\n", currentDir)
		fmt.Println(physfs.GetError())
	} else {
		fmt.Printf("Unmounting directory \"%v\" [OK]\n", currentDir)
	}

	// Find zip files and unmount it
	files, err = os.ReadDir(".")
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".zip") {
			// Open the file
			if !physfs.Unmount(file.Name()) {
				fmt.Printf("Unmounting %v [FAIL]\n", file.Name())
				fmt.Println(physfs.GetError())
			} else {
				fmt.Printf("Unmounting %v [OK]\n", file.Name())
			}
		}
	}
}

// Loops through given comand line arguments and processes them for later use by the game
func processCommandLine() {
	// If there are command line arguments
	if len(os.Args[1:]) > 0 {
		sys.cmdFlags = make(map[string]string)
		key := ""
		player := 1
		r1, _ := regexp.Compile("^-[h%?]$")
		r2, _ := regexp.Compile("^-")
		// Loop through arguments
		for _, a := range os.Args[1:] {
			// If getting help about command line options
			if r1.MatchString(a) {
				text := `Options (case sensitive):
-h -?                   Help
-log <logfile>          Records match data to <logfile>
-r <path>               Loads motif <path>. eg. -r motifdir or -r motifdir/system.def
-lifebar <path>         Loads lifebar <path>. eg. -lifebar data/fight.def
-storyboard <path>      Loads storyboard <path>. eg. -storyboard chars/kfm/intro.def
-width <num>            Overrides game window width
-height <num>           Overrides game window height

Quick VS Options:
-p<n> <playername>      Loads player n, eg. -p3 kfm
-p<n>.ai <level>        Sets player n's AI to <level>, eg. -p1.ai 8
-p<n>.color <col>       Sets player n's color to <col>
-p<n>.power <power>     Sets player n's power to <power>
-p<n>.life <life>       Sets player n's life to <life>
-tmode1 <tmode>         Sets p1 team mode to <tmode>
-tmode2 <tmode>         Sets p2 team mode to <tmode>
-time <num>             Round time (-1 to disable)
-rounds <num>           Plays for <num> rounds, and then quits
-s <stagename>          Loads stage <stagename>

Extra Options (by LeonKasovan):
-game <gamename>        Change directory to gamename
-updatechar             Update character def in select.def based on chars directory
-updatestage            Update stage def in select.def based on stages directory
-validate               Validate game assets existance
-fix                    Fix game assets naming (into lowercase)
-install                Install default screenpack
-allstage               Auto load all stages
-allchar				Auto load all characters

Debug Options:
-nojoy                  Disables joysticks
-nomusic                Disables music
-nosound                Disables all sound effects and music
-windowed               Windowed mode (disables fullscreen)
-togglelifebars         Disables display of the Life and Power bars
-maxpowermode           Enables auto-refill of Power bars
-ailevel <level>        Changes game difficulty setting to <level> (1-8)
-speed <speed>          Changes game speed setting to <speed> (10%%-200%%)
-stresstest <frameskip> Stability test (AI matches at speed increased by <frameskip>)
-speedtest              Speed test (match speed x100)`
				//ShowInfoDialog(text, "I.K.E.M.E.N Command line options")
				fmt.Printf("I.K.E.M.E.N Command line options\n\n" + text + "\nPress ENTER to exit")
				var s string
				fmt.Scanln(&s)
				os.Exit(0)
				// If a control argument starting with - (eg. -p3, -s, -rounds)
			} else if r2.MatchString(a) {
				// Set a blank value for the key to start with
				sys.cmdFlags[a] = ""
				// Prepare the key for the next argument
				key = a
				// If an argument with no key
			} else if key == "" {
				// Set p1/p2's name
				sys.cmdFlags[fmt.Sprintf("-p%v", player)] = a
				player += 1
				// If a key is prepared for this argument
			} else {
				// Set the argument for this key
				sys.cmdFlags[key] = a
				key = ""
			}
		}
	}
}
