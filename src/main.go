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

	lua "github.com/yuin/gopher-lua"
)

var Version = "development"
var BuildTime = ""

//go:embed assets.zip
var assetsZip []byte

//go:embed screenpack.zip
var screenpackZip []byte

func renameFilesToLowerCase(root string) error {
	// Walk through the directory recursively
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get the directory and the lowercase file name
		dir := filepath.Dir(path)
		lowercaseName := strings.ToLower(info.Name())

		// Check if renaming is needed
		if info.Name() != lowercaseName {
			newPath := filepath.Join(dir, lowercaseName)

			// Rename the file
			err := os.Rename(path, newPath)
			if err != nil {
				return fmt.Errorf("failed to rename %s to %s: %w", path, newPath, err)
			}
			fmt.Printf("Renamed: %s -> %s\n", path, newPath)
		}
		return nil
	})
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
		outFile, err := os.Create(file.Name)
		if err != nil {
			return err
		}
		defer outFile.Close()

		// Copy the file contents to the destination file
		_, err = io.Copy(outFile, fileReader)
		if err != nil {
			return err
		}

		// fmt.Printf("Successfully extracted: %s\n", file.Name)
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
	destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
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
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	// Open or create the file
	file2, err := os.Create(filename + ".update")
	if err != nil {
		file.Close()
		return err
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
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	// Open or create the file
	file2, err := os.Create(filename + ".update")
	if err != nil {
		file.Close()
		return err
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
					// fmt.Printf("scanner.Text=%v path_sep1=%v path_sep2=%v\n", scanner.Text(), path_sep1, path_sep2)
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

func createLog(p string) *os.File {
	f, err := os.Create(p)
	if err != nil {
		panic(err)
	}
	return f
}
func closeLog(f *os.File) {
	f.Close()
}

func main() {
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
	_, err1 := os.Stat("external")
	_, err2 := os.Stat("data/mugen.cfg")
	if os.IsNotExist(err1) && err2 == nil {
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

	// Try reading stats
	if _, err := os.ReadFile("save/stats.json"); err != nil {
		// If there was an error reading, write an empty json file
		f, err := os.Create("save/stats.json")
		chk(err)
		f.Write([]byte("{}"))
		chk(f.Close())
	}

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
		if FileExist("external/script/audit.lua") == "" {
			err := extractFileFromEmbed(assetsZip, "external/script/audit.lua")
			if err != nil {
				fmt.Printf("[main.go] Error extracting audit.lua: %v\n", err)
				os.Exit(0)
			}
		}
		if FileExist("external/script/json.lua") == "" {
			err := extractFileFromEmbed(assetsZip, "external/script/json.lua")
			if err != nil {
				fmt.Printf("[main.go] Error extracting json.lua: %v\n", err)
				os.Exit(0)
			}
		}
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
	if ftemp, err1 := os.Open(sys.cfg.Config.System); err1 != nil {
		ftemp.Close()
		var err2 = Error(
			"Main lua file \"" + sys.cfg.Config.System + "\" error." +
				"\n" + err1.Error(),
		)
		ShowErrorDialog(err2.Error())
		panic(err2)
	} else {
		ftemp.Close()
	}

	// Initialize game and create window
	sys.luaLState = sys.init(sys.gameWidth, sys.gameHeight)
	defer sys.shutdown()

	// Begin processing game using its lua scripts
	no := 1
	if err := sys.luaLState.DoFile(sys.cfg.Config.System); err != nil {
		fmt.Printf("[%v]Error: %v\n", err, no)
		// Display error logs.
		errorLog := createLog("Ikemen.log")
		// defer closeLog(errorLog)
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
