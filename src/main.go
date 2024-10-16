package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	_ "embed" // Support for go:embed resources
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

var Version = "202408-dev"
var BuildTime = "2024.08.17"

//go:embed assets.zip
var assetsZip []byte

//go:embed screenpack.zip
var screenpackZip []byte

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
func chkEX(err error, txt string) {
	if err != nil {
		ShowErrorDialog(txt + err.Error())
		panic(Error(txt + err.Error()))
	}
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
	filename := NormalizeFile(fname)
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
	filename := NormalizeFile(fname)
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
func fixConfig(fname string) error {
	// Open the file
	filename := NormalizeFile(fname)
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	// Open or create the file
	file2, err := os.Create(NormalizeFile("save/config.fix.json"))
	if err != nil {
		file.Close()
		return err
	}

	// Create a buffered writer
	writer := bufio.NewWriter(file2)

	re1 := regexp.MustCompile(`"CommonAir": "(\S+)",`)
	re2 := regexp.MustCompile(`"CommonCmd": "(\S+)",`)
	re3 := regexp.MustCompile(`"CommonConst": "(\S+)",`)

	// Create a new scanner
	scanner := bufio.NewScanner(file)

	// Loop through each line
	var result []string
	for scanner.Scan() {
		result = re1.FindStringSubmatch(scanner.Text())
		if result != nil {
			writer.WriteString(fmt.Sprintf("\"CommonAir\": [\"%v\"],\n", result[1]))
			continue
		}
		result = re2.FindStringSubmatch(scanner.Text())
		if result != nil {
			writer.WriteString(fmt.Sprintf("\"CommonCmd\": [\"%v\"],\n", result[1]))
			continue
		}
		result = re3.FindStringSubmatch(scanner.Text())
		if result != nil {
			// fmt.Printf("[main.go][fixConfig]1: %v\n", scanner.Text())
			// fmt.Printf("[main.go][fixConfig]2: %v\n", fmt.Sprintf("\"CommonConst\": [\"%v\"],\n", result[1]))
			writer.WriteString(fmt.Sprintf("\"CommonConst\": [\"%v\"],\n", result[1]))
			continue
		}
		if strings.Contains(scanner.Text(), "external/shaders/") {
			writer.WriteString(fmt.Sprintf("\n"))
			continue
		}
		if strings.Contains(scanner.Text(), "MSAA") {
			writer.WriteString(fmt.Sprintf("  \"MSAA\": false,\n"))
			continue
		}
		if strings.Contains(scanner.Text(), "PostProcessingShader") {
			writer.WriteString(fmt.Sprintf("  \"PostProcessingShader\": 0,\n"))
			continue
		}
		writer.WriteString(scanner.Text() + "\n")
	}
	writer.Flush()
	file.Close()
	file2.Close()
	os.Rename(filename, NormalizeFile("save/config.bak.json"))
	os.Rename(NormalizeFile("save/config.fix.json"), filename)
	return scanner.Err()
}
func main() {
	is_mugen_game := false
	fmt.Printf("[main.go][main] Running at OS=[%v] ARCH=[%v]\n", runtime.GOOS, runtime.GOARCH)

	// Check if the "external" directory exists and data/mugen.cfg, if not exists then extract assets from embedded
	_, err1 := os.Stat("external")
	_, err2 := os.Stat("data/mugen.cfg")
	if os.IsNotExist(err1) && err2 == nil {
		err := extractEmbed(assetsZip)
		if err != nil {
			fmt.Printf("[main.go][setupConfig] Error extracting asset: %v\n", err)
		}
		fmt.Println("[main.go][main] Mugen Game detected. Assets extraction completed successfully.")
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

	// Make save directories, if they don't exist
	os.Mkdir("save", os.ModeSticky|0755)
	os.Mkdir("save/replays", os.ModeSticky|0755)

	// Try reading stats
	if _, err := os.ReadFile("save/stats.json"); err != nil {
		// If there was an error reading, write an empty json file
		f, err1 := os.Create("save/stats.json")
		chk(err1)
		f.Write([]byte("{}"))
		chk(f.Close())
	}

	// Setup config values, and get a reference to the config object for the main script and window size
	tmp := setupConfig(is_mugen_game)

	//os.Mkdir("debug", os.ModeSticky|0755)

	// Check if the main lua file exists.
	if ftemp, err := os.Open(tmp.System); err != nil {
		ftemp.Close()
		var err1 = Error(
			"Main lua file \"" + tmp.System + "\" error." +
				"\n" + err.Error(),
		)
		ShowErrorDialog(err1.Error())
		panic(err1)
	} else {
		ftemp.Close()
	}

	// Initialize game and create window
	sys.luaLState = sys.init(tmp.GameWidth, tmp.GameHeight)
	defer sys.shutdown()

	// Begin processing game using its lua scripts
	fmt.Printf("[main.go][main]: Running in lua script=[%v] using motif=[%v]\n", tmp.System, tmp.Motif)
	if err := sys.luaLState.DoFile(tmp.System); err != nil {
		// Display error logs.
		errorLog := createLog("Ikemen.log")
		defer closeLog(errorLog)
		fmt.Fprintln(errorLog, err)
		switch err.(type) {
		case *lua.ApiError:
			errstr := strings.Split(err.Error(), "\n")[0]
			if len(errstr) < 10 || errstr[len(errstr)-10:] != "<game end>" {
				ShowErrorDialog(fmt.Sprintf("%v\n\nError saved to Ikemen.log", err))
				panic(err)
			}
		default:
			ShowErrorDialog(fmt.Sprintf("%v\n\nError saved to Ikemen.log", err))
			panic(err)
		}
	}
	// fmt.Printf("[main.go][setupConfig] Joystick Setting Updated from options.lua\n")
	// for _, jc := range tmp.JoystickConfig {
	// 	fmt.Printf("sys.joystickConfig=%v [%v]\n", jc.Joystick, jc.Buttons)
	// }
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

Extra Feature (by leonkasovan):
-updatechar             Add new characters from [chars] directory into select.def
-updatestage            Add new stages from [stages] directory into select.def
-install                Install default screenpack and Ikemen's assets
-audit                  Verify (and fix) integrity of assets included in definition files

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

type configSettings struct {
	AIRamping                     bool
	AIRandomColor                 bool
	AISurvivalColor               bool
	AudioDucking                  bool
	AudioSampleRate               int32
	AutoGuard                     bool
	BarGuard                      bool
	BarRedLife                    bool
	BarStun                       bool
	Borderless                    bool
	CommonAir                     []string
	CommonCmd                     []string
	CommonConst                   []string
	CommonFx                      []string
	CommonLua                     []string
	CommonStates                  []string
	ControllerStickSensitivitySDL int16
	ControllerStickSensitivity    float32
	Credits                       int
	DebugClipboardRows            int
	DebugClsnDarken               bool
	DebugConsoleRows              int
	DebugFont                     string
	DebugFontScale                float32
	DebugKeys                     bool
	DebugMode                     bool
	Difficulty                    int
	EscOpensMenu                  bool
	ExternalShaders               []string
	FirstRun                      bool
	FontShaderVer                 uint
	ForceStageZoomin              float32
	ForceStageZoomout             float32
	Framerate                     int32
	Fullscreen                    bool
	FullscreenRefreshRate         int32
	FullscreenWidth               int32
	FullscreenHeight              int32
	GameWidth                     int32
	GameHeight                    int32
	GameFramerate                 float32
	GameSpeed                     float32
	InputButtonAssist             bool
	InputSOCDResolution           int32
	IP                            map[string]string
	KeepAspect                    bool
	WindowScaleMode               bool
	Language                      string
	LifeMul                       float32
	ListenPort                    string
	LoseSimul                     bool
	LoseTag                       bool
	MaxAfterImage                 int32
	MaxBgmVolume                  int
	MaxDrawGames                  int32
	MaxExplod                     int
	MaxHelper                     int32
	MaxPlayerProjectile           int
	Modules                       []string
	Motif                         string
	MSAA                          int32
	NumSimul                      [2]int
	NumTag                        [2]int
	NumTurns                      [2]int
	PanningRange                  float32
	PauseMasterVolume             int
	Players                       int
	PngSpriteFilter               bool
	PostProcessingShader          int32
	QuickContinue                 bool
	RatioAttack                   [4]float32
	RatioLife                     [4]float32
	RatioRecoveryBase             float32
	RatioRecoveryBonus            float32
	RoundsNumSimul                int32
	RoundsNumSingle               int32
	RoundsNumTag                  int32
	RoundTime                     int32
	ScreenshotFolder              string
	StartStage                    string
	StereoEffects                 bool
	System                        string
	Team1VS2Life                  float32
	TeamDuplicates                bool
	TeamLifeShare                 bool
	TeamPowerShare                bool
	TrainingChar                  string
	TurnsRecoveryBase             float32
	TurnsRecoveryBonus            float32
	VolumeBgm                     int
	VolumeMaster                  int
	VolumeSfx                     int
	VRetrace                      int
	WavChannels                   int32
	WindowCentered                bool
	WindowIcon                    []string
	WindowTitle                   string
	XinputTriggerSensitivity      float32
	ZoomActive                    bool
	ZoomDelay                     bool
	ZoomSpeed                     float32
	KeyConfig                     []struct {
		Joystick int
		Buttons  []interface{}
	}
	JoystickConfig []struct {
		Joystick int
		Buttons  []interface{}
	}
	JoystickDefaultConfig []struct {
		JoystickName string
		Buttons      []string
	}
}

//go:embed resources/defaultConfig.json
var defaultConfig []byte

// Sets default config settings, then attemps to load existing config from disk
func setupConfig(is_mugen_game bool) configSettings {
	Atoi := func(key string) int {
		if i, err := strconv.Atoi(key); err == nil {
			return i
		}
		return 999
	}
	// Unmarshal default config string into a struct
	tmp := configSettings{}
	chk(json.Unmarshal(defaultConfig, &tmp))
	// fmt.Printf("[main.go][setupConfig] using embedded defaultConfig.json\ntmp.JoystickConfig[0]: %v\ntmp.JoystickConfig[1]: %v\ntmp.JoystickConfig[2]: %v\n", tmp.JoystickConfig[0], tmp.JoystickConfig[1], tmp.JoystickConfig[2])
	// Config file path
	cfgPath := NormalizeFile("save/config.json")
	// If a different config file is defined in the command line parameters, use it instead
	if _, ok := sys.cmdFlags["-config"]; ok {
		cfgPath = sys.cmdFlags["-config"]
	}
	// Load the config file, overwriting the defaults
	if FileExist(cfgPath) != "" {
		counter := 0
		for {
			if bytes, err := os.ReadFile(cfgPath); err == nil {
				if len(bytes) >= 3 &&
					bytes[0] == 0xef && bytes[1] == 0xbb && bytes[2] == 0xbf {
					bytes = bytes[3:]
				}
				if json.Unmarshal(bytes, &tmp) != nil {
					fmt.Printf("[main.go] setupConfig fix %v\n", cfgPath)
					if err := fixConfig(cfgPath); err != nil {
						ShowErrorDialog(err.Error())
						panic(err)
					}
				} else {
					counter = 1
				}
				counter = counter + 1
				if counter > 1 {
					break
				}
			}
		}
	}
	// fmt.Printf("[main.go][setupConfig] Assigning Joystick default setting\n")
	sys.joystickDefaultConfig = map[string]KeyConfig{} // Initialize empty map for KeyConfig
	for _, jc := range tmp.JoystickDefaultConfig {
		// fmt.Printf("sys.joystickDefaultConfig[%v]=[%v] %v\n", jc.JoystickName, id, jc.Buttons)
		b := jc.Buttons
		sys.joystickDefaultConfig[jc.JoystickName] = KeyConfig{0,
			Atoi(b[0]), Atoi(b[1]), Atoi(b[2]),
			Atoi(b[3]), Atoi(b[4]), Atoi(b[5]),
			Atoi(b[6]), Atoi(b[7]), Atoi(b[8]),
			Atoi(b[9]), Atoi(b[10]), Atoi(b[11]),
			Atoi(b[12]), Atoi(b[13])}
	}
	// fmt.Printf("[main.go][setupConfig] after loading config.json\n")
	// for id, jc := range tmp.JoystickConfig {
	// 	fmt.Printf("tmp.JoystickConfig[%v]: %v\n", id, jc)
	// }
	// Fix incorrect settings (default values saved into config.json)
	switch tmp.AudioSampleRate {
	case 22050, 44100, 48000:
	default:
		tmp.AudioSampleRate = 44100
	}
	tmp.Framerate = Clamp(tmp.Framerate, 1, 840)
	tmp.PauseMasterVolume = int(Clamp(int32(tmp.PauseMasterVolume), 0, 100))
	tmp.MaxBgmVolume = int(Clamp(int32(tmp.MaxBgmVolume), 100, 250))
	tmp.NumSimul[0] = int(Clamp(int32(tmp.NumSimul[0]), 2, int32(MaxSimul)))
	tmp.NumSimul[1] = int(Clamp(int32(tmp.NumSimul[1]), int32(tmp.NumSimul[0]), int32(MaxSimul)))
	tmp.NumTag[0] = int(Clamp(int32(tmp.NumTag[0]), 2, int32(MaxSimul)))
	tmp.NumTag[1] = int(Clamp(int32(tmp.NumTag[1]), int32(tmp.NumTag[0]), int32(MaxSimul)))
	tmp.PanningRange = ClampF(tmp.PanningRange, 0, 100)
	tmp.Players = int(Clamp(int32(tmp.Players), 1, int32(MaxSimul)*2))
	tmp.WavChannels = Clamp(tmp.WavChannels, 1, 256)

	//Import Mugen setting
	if is_mugen_game {
		fmt.Printf("[main.go][setupConfig] import data/mugen.cfg\n")
		file, err := os.Open("data/mugen.cfg")
		if err != nil {
			fmt.Printf("[main.go][setupConfig] Error loading data/mugen.cfg\n")
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var result []string
		var line string
		for scanner.Scan() {
			line = scanner.Text()
			if len(line) < 1 {
				continue
			}
			if line[0] == ';' {
				continue
			}
			result = regexp.MustCompile(`[Mm]otif\s*=\s*(\S+)`).FindStringSubmatch(line)
			if result != nil {
				tmp.Motif = strings.ReplaceAll(result[1], "\\", "/")
				fmt.Printf("[main.go][setupConfig] Import Motif=%v\n", tmp.Motif)
				continue
			}
			result = regexp.MustCompile(`[Ss]tart[Ss]tage\s*=\s*(\S+)`).FindStringSubmatch(line)
			if result != nil {
				tmp.StartStage = strings.ReplaceAll(result[1], "\\", "/")
				fmt.Printf("[main.go][setupConfig] Import StartStage=%v\n", tmp.StartStage)
				continue
			}
			result = regexp.MustCompile(`[Gg]ame[Ww]idth\s*=\s*(\d+)`).FindStringSubmatch(line)
			if result != nil {
				tmp.GameWidth = int32(Atoi(result[1]))
				fmt.Printf("[main.go][setupConfig] Import GameWidth=%v\n", tmp.GameWidth)
				continue
			}
			result = regexp.MustCompile(`[Gg]ame[Hh]eight\s*=\s*(\d+)`).FindStringSubmatch(line)
			if result != nil {
				tmp.GameHeight = int32(Atoi(result[1]))
				fmt.Printf("[main.go][setupConfig] Import GameHeight=%v\n", tmp.GameHeight)
				continue
			}
		}
	} else {
		fmt.Printf("[main.go][setupConfig] NOT importing data/mugen.cfg\n")
	}

	// Save config file, indent with two spaces to match calls to json.encode() in the Lua code
	cfg, _ := json.MarshalIndent(tmp, "", "  ")
	chk(os.WriteFile(cfgPath, cfg, 0644))

	// If given width/height arguments, override config's width/height here
	if _, wok := sys.cmdFlags["-width"]; wok {
		var w, _ = strconv.ParseInt(sys.cmdFlags["-width"], 10, 32)
		tmp.GameWidth = int32(w)
	}
	if _, hok := sys.cmdFlags["-height"]; hok {
		var h, _ = strconv.ParseInt(sys.cmdFlags["-height"], 10, 32)
		tmp.GameHeight = int32(h)
	}

	// Set each config property to the system object
	sys.afterImageMax = tmp.MaxAfterImage
	sys.allowDebugKeys = tmp.DebugKeys
	sys.allowDebugMode = tmp.DebugMode
	sys.audioDucking = tmp.AudioDucking
	sys.audioSampleRate = tmp.AudioSampleRate
	sys.bgmVolume = tmp.VolumeBgm
	sys.maxBgmVolume = tmp.MaxBgmVolume
	sys.borderless = tmp.Borderless
	sys.cam.ZoomDelayEnable = tmp.ZoomDelay
	sys.cam.ZoomActive = tmp.ZoomActive
	sys.cam.ZoomMax = tmp.ForceStageZoomin
	sys.cam.ZoomMin = tmp.ForceStageZoomout
	sys.cam.ZoomSpeed = 12 - tmp.ZoomSpeed
	sys.commonAir = tmp.CommonAir
	sys.commonCmd = tmp.CommonCmd
	sys.commonConst = tmp.CommonConst
	sys.commonFx = tmp.CommonFx
	sys.commonLua = tmp.CommonLua
	sys.commonStates = tmp.CommonStates
	sys.clipboardRows = tmp.DebugClipboardRows
	sys.clsnDarken = tmp.DebugClsnDarken
	sys.consoleRows = tmp.DebugConsoleRows
	sys.controllerStickSensitivityGLFW = tmp.ControllerStickSensitivity
	sys.controllerStickSensitivitySDL = tmp.ControllerStickSensitivitySDL
	sys.explodMax = tmp.MaxExplod
	sys.externalShaderList = tmp.ExternalShaders
	// Bump up shader version for macOS only
	if runtime.GOOS == "darwin" {
		tmp.FontShaderVer = max(150, tmp.FontShaderVer)
	}
	sys.fontShaderVer = tmp.FontShaderVer
	// Resoluion stuff
	sys.fullscreen = tmp.Fullscreen
	sys.fullscreenRefreshRate = tmp.FullscreenRefreshRate
	sys.fullscreenWidth = tmp.FullscreenWidth
	sys.fullscreenHeight = tmp.FullscreenHeight
	FPS = int(tmp.Framerate)
	sys.gameWidth = tmp.GameWidth
	sys.gameHeight = tmp.GameHeight
	sys.gameSpeed = tmp.GameFramerate / float32(tmp.Framerate)
	sys.keepAspect = tmp.KeepAspect
	sys.windowScaleMode = tmp.WindowScaleMode
	sys.helperMax = tmp.MaxHelper
	sys.inputButtonAssist = tmp.InputButtonAssist
	sys.inputSOCDresolution = Clamp(tmp.InputSOCDResolution, 0, 4)
	sys.language = tmp.Language
	sys.lifeMul = tmp.LifeMul / 100
	sys.lifeShare = [...]bool{tmp.TeamLifeShare, tmp.TeamLifeShare}
	sys.listenPort = tmp.ListenPort
	sys.loseSimul = tmp.LoseSimul
	sys.loseTag = tmp.LoseTag
	sys.masterVolume = tmp.VolumeMaster
	if tmp.MSAA <= -1 {
		tmp.MSAA = 0
	}
	sys.multisampleAntialiasing = tmp.MSAA
	sys.pauseMasterVolume = tmp.PauseMasterVolume
	sys.panningRange = tmp.PanningRange
	sys.playerProjectileMax = tmp.MaxPlayerProjectile
	sys.postProcessingShader = tmp.PostProcessingShader
	sys.pngFilter = tmp.PngSpriteFilter
	sys.powerShare = [...]bool{tmp.TeamPowerShare, tmp.TeamPowerShare}
	tmp.ScreenshotFolder = strings.TrimSpace(tmp.ScreenshotFolder)
	if tmp.ScreenshotFolder != "" {
		tmp.ScreenshotFolder = strings.Replace(tmp.ScreenshotFolder, "\\", "/", -1)
		tmp.ScreenshotFolder = strings.TrimRight(tmp.ScreenshotFolder, "/")
		sys.screenshotFolder = tmp.ScreenshotFolder + "/"
	} else {
		sys.screenshotFolder = tmp.ScreenshotFolder
	}
	sys.stereoEffects = tmp.StereoEffects
	sys.team1VS2Life = tmp.Team1VS2Life / 100
	sys.vRetrace = tmp.VRetrace
	sys.wavChannels = tmp.WavChannels
	sys.wavVolume = tmp.VolumeSfx
	sys.windowCentered = tmp.WindowCentered
	sys.windowMainIconLocation = tmp.WindowIcon
	sys.windowTitle = tmp.WindowTitle
	sys.xinputTriggerSensitivity = tmp.XinputTriggerSensitivity
	stoki := func(key string) int {
		return int(StringToKey(key))
	}
	for _, kc := range tmp.KeyConfig {
		b := kc.Buttons
		sys.keyConfig = append(sys.keyConfig, KeyConfig{kc.Joystick,
			stoki(b[0].(string)), stoki(b[1].(string)), stoki(b[2].(string)),
			stoki(b[3].(string)), stoki(b[4].(string)), stoki(b[5].(string)),
			stoki(b[6].(string)), stoki(b[7].(string)), stoki(b[8].(string)),
			stoki(b[9].(string)), stoki(b[10].(string)), stoki(b[11].(string)),
			stoki(b[12].(string)), stoki(b[13].(string))})
	}
	fmt.Printf("[main.go][setupConfig] Assigning Joystick setting to Engine\n")
	if _, ok := sys.cmdFlags["-nojoy"]; !ok {
		for _, jc := range tmp.JoystickConfig {
			fmt.Printf("sys.joystickConfig[%v] = %v\n", jc.Joystick, jc.Buttons)
			b := jc.Buttons
			sys.joystickConfig = append(sys.joystickConfig, KeyConfig{jc.Joystick,
				Atoi(b[0].(string)), Atoi(b[1].(string)), Atoi(b[2].(string)),
				Atoi(b[3].(string)), Atoi(b[4].(string)), Atoi(b[5].(string)),
				Atoi(b[6].(string)), Atoi(b[7].(string)), Atoi(b[8].(string)),
				Atoi(b[9].(string)), Atoi(b[10].(string)), Atoi(b[11].(string)),
				Atoi(b[12].(string)), Atoi(b[13].(string))})
		}
	}

	if _, ok := sys.cmdFlags["-updatechar"]; ok {
		fmt.Printf("[main.go][setupConfig] Update data/select.def based on [char] directory\n")
		err := updateCharInSelectDef(NormalizeFile("data/select.def"))
		if err != nil {
			fmt.Printf("[main.go][setupConfig] %v\n", err)
		}
	}

	if _, ok := sys.cmdFlags["-updatestage"]; ok {
		fmt.Printf("[main.go][setupConfig] Update data/select.def based on [stages] directory\n")
		err := updateStageInSelectDef(NormalizeFile("data/select.def"))
		if err != nil {
			fmt.Printf("[main.go][setupConfig] %v\n", err)
		}
	}

	if _, ok := sys.cmdFlags["-install"]; ok {
		fmt.Printf("[main.go][setupConfig] Install default screenpack\n")
		err := extractEmbed(screenpackZip)
		if err != nil {
			fmt.Printf("[main.go][setupConfig] Error extracting screenpack: %v\n", err)
		}
		err = extractEmbed(assetsZip)
		if err != nil {
			fmt.Printf("[main.go][setupConfig] Error extracting asset: %v\n", err)
		}
	}

	if _, ok := sys.cmdFlags["-audit"]; ok {
		if FileExist("external/script/audit.lua") == "" {
			err := extractFileFromEmbed(assetsZip, "external/script/audit.lua")
			if err != nil {
				fmt.Printf("[main.go][setupConfig] Error extracting audit.lua: %v\n", err)
				os.Exit(0)
			}
		}
		if FileExist("external/script/json.lua") == "" {
			err := extractFileFromEmbed(assetsZip, "external/script/json.lua")
			if err != nil {
				fmt.Printf("[main.go][setupConfig] Error extracting json.lua: %v\n", err)
				os.Exit(0)
			}
		}
		l := lua.NewState()
		l.Options.IncludeGoStackTrace = true
		l.OpenLibs()
		systemScriptInit(l)
		fmt.Printf("\n\n==================================\nVerifying included the game assets...\n")
		if err := l.DoFile("external/script/audit.lua"); err != nil {
			fmt.Printf("[main.go][setupConfig] Error running audit script: %v\n", err)
		}
		os.Exit(0)
	}

	return tmp
}
