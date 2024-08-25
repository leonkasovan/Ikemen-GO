package main

import (
	"archive/zip"
	"bufio"
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

var Version = "PSC-dev"
var BuildTime = "2024.08.17"

//go:embed assets.zip
var assetsZip []byte

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
func fixConfig(filename string) error {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Open or create the file
	file2, err := os.Create("save/config.fix.json")
	if err != nil {
		return err
	}
	defer file2.Close()

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
			// fmt.Printf("[DEBUG][main.go][fixConfig]1: %v\n", scanner.Text())
			// fmt.Printf("[DEBUG][main.go][fixConfig]2: %v\n", fmt.Sprintf("\"CommonConst\": [\"%v\"],\n", result[1]))
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
	os.Rename(filename, "save/config.bak.json")
	os.Rename("save/config.fix.json", filename)
	return scanner.Err()
}
func main() {
	fmt.Printf("[DEBUG][main.go][main] Running at OS=[%v] ARCH=[%v]\n", runtime.GOOS, runtime.GOARCH)

	// Check if the "external" directory exists and data/mugen.cfg, if not exists then extract assets from embedded
	_, err1 := os.Stat("external")
	_, err2 := os.Stat("data/mugen.cfg")
	// fmt.Printf("[DEBUG][main.go][main] err1=[%v] err2=[%v]\n", err1, err2)

	if os.IsNotExist(err1) && err2 == nil {
		// Create a temporary file to hold the embedded ZIP data
		tmpZipPath := "assets_temp.zip"
		err := os.WriteFile(tmpZipPath, assetsZip, 0644)
		if err != nil {
			fmt.Printf("[DEBUG][main.go][main] Failed to write temp ZIP file: %v\n", err)
			return
		}
		defer os.Remove(tmpZipPath) // Clean up the temp file after extraction

		// Open the ZIP file
		zipReader, err := zip.OpenReader(tmpZipPath)
		if err != nil {
			fmt.Printf("[DEBUG][main.go][main] Failed to open ZIP file: %v\n", err)
			return
		}
		defer zipReader.Close()

		// Iterate over each file in the ZIP archive
		for _, file := range zipReader.File {
			filePath := file.Name

			if file.FileInfo().IsDir() {
				// Create directories
				os.MkdirAll(filePath, os.ModePerm)
				continue
			}

			// Extract the file
			if err := extractFile(file, filePath); err != nil {
				fmt.Printf("[DEBUG][main.go][main] Failed to extract file: %v\n", err)
				return
			}
		}

		fmt.Println("[DEBUG][main.go][main] Mugen Game detected. Assets extraction completed successfully.")
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
	tmp := setupConfig()

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
	fmt.Printf("[DEBUG][main.go][main]: Running in lua script=[%v]\n", tmp.System)
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
	fmt.Printf("[DEBUG][main.go][setupConfig] Joystick Setting Updated from options.lua\n")
	for _, jc := range tmp.JoystickConfig {
		fmt.Printf("sys.joystickConfig=%v [%v]\n", jc.Joystick, jc.Buttons)
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

type configSettings struct {
	AIRamping                  bool
	AIRandomColor              bool
	AISurvivalColor            bool
	AudioDucking               bool
	AudioSampleRate            int32
	AutoGuard                  bool
	BarGuard                   bool
	BarRedLife                 bool
	BarStun                    bool
	Borderless                 bool
	CommonAir                  []string
	CommonCmd                  []string
	CommonConst                []string
	CommonFx                   []string
	CommonLua                  []string
	CommonStates               []string
	ControllerStickSensitivity float32
	Credits                    int
	DebugClipboardRows         int
	DebugClsnDarken            bool
	DebugConsoleRows           int
	DebugFont                  string
	DebugFontScale             float32
	DebugKeys                  bool
	DebugMode                  bool
	Difficulty                 int
	EscOpensMenu               bool
	ExternalShaders            []string
	FirstRun                   bool
	FontShaderVer              uint
	ForceStageZoomin           float32
	ForceStageZoomout          float32
	Framerate                  int32
	Fullscreen                 bool
	FullscreenRefreshRate      int32
	FullscreenWidth            int32
	FullscreenHeight           int32
	GameWidth                  int32
	GameHeight                 int32
	GameFramerate              float32
	InputButtonAssist          bool
	InputSOCDResolution        int32
	IP                         map[string]string
	LifeMul                    float32
	ListenPort                 string
	LoseSimul                  bool
	LoseTag                    bool
	MaxAfterImage              int32
	MaxBgmVolume               int
	MaxDrawGames               int32
	MaxExplod                  int
	MaxHelper                  int32
	MaxPlayerProjectile        int
	Modules                    []string
	Motif                      string
	MSAA                       bool
	NumSimul                   [2]int
	NumTag                     [2]int
	NumTurns                   [2]int
	PanningRange               float32
	Players                    int
	PngSpriteFilter            bool
	PostProcessingShader       int32
	QuickContinue              bool
	RatioAttack                [4]float32
	RatioLife                  [4]float32
	RatioRecoveryBase          float32
	RatioRecoveryBonus         float32
	RoundsNumSimul             int32
	RoundsNumSingle            int32
	RoundsNumTag               int32
	RoundTime                  int32
	ScreenshotFolder           string
	StartStage                 string
	StereoEffects              bool
	System                     string
	Team1VS2Life               float32
	TeamDuplicates             bool
	TeamLifeShare              bool
	TeamPowerShare             bool
	TrainingChar               string
	TurnsRecoveryBase          float32
	TurnsRecoveryBonus         float32
	VolumeBgm                  int
	VolumeMaster               int
	VolumeSfx                  int
	VRetrace                   int
	WavChannels                int32
	WindowCentered             bool
	WindowIcon                 []string
	WindowTitle                string
	XinputTriggerSensitivity   float32
	ZoomActive                 bool
	ZoomDelay                  bool
	ZoomSpeed                  float32
	KeyConfig                  []struct {
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
func setupConfig() configSettings {
	Atoi := func(key string) int {
		if i, err := strconv.Atoi(key); err == nil {
			return i
		}
		return 999
	}
	// Unmarshal default config string into a struct
	tmp := configSettings{}
	chk(json.Unmarshal(defaultConfig, &tmp))
	fmt.Printf("[DEBUG][main.go][setupConfig] Assigning Joystick default setting\n")
	sys.joystickDefaultConfig = map[string]KeyConfig{} // Initialize empty map for KeyConfig
	for id, jc := range tmp.JoystickDefaultConfig {
		fmt.Printf("sys.joystickDefaultConfig[%v]=[%v] %v\n", jc.JoystickName, id, jc.Buttons)
		b := jc.Buttons
		sys.joystickDefaultConfig[jc.JoystickName] = KeyConfig{0,
			Atoi(b[0]), Atoi(b[1]), Atoi(b[2]),
			Atoi(b[3]), Atoi(b[4]), Atoi(b[5]),
			Atoi(b[6]), Atoi(b[7]), Atoi(b[8]),
			Atoi(b[9]), Atoi(b[10]), Atoi(b[11]),
			Atoi(b[12]), Atoi(b[13])}
	}
	fmt.Printf("[DEBUG][main.go][setupConfig] using embedded defaultConfig,json\ntmp.JoystickConfig[0]: %v\ntmp.JoystickConfig[1]: %v\ntmp.JoystickConfig[2]: %v\n", tmp.JoystickConfig[0], tmp.JoystickConfig[1], tmp.JoystickConfig[2])
	// Config file path
	cfgPath := "save/config.json"
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
					fmt.Printf("[DEBUG][main.go] setupConfig fix %v\n", cfgPath)
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
	fmt.Printf("[DEBUG][main.go][setupConfig] after loading config.json\ntmp.JoystickConfig[0]: %v\ntmp.JoystickConfig[1]: %v\ntmp.JoystickConfig[2]: %v\n", tmp.JoystickConfig[0], tmp.JoystickConfig[1], tmp.JoystickConfig[2])
	// Fix incorrect settings (default values saved into config.json)
	switch tmp.AudioSampleRate {
	case 22050, 44100, 48000:
	default:
		tmp.AudioSampleRate = 44100
	}
	tmp.Framerate = Clamp(tmp.Framerate, 1, 840)
	tmp.MaxBgmVolume = int(Clamp(int32(tmp.MaxBgmVolume), 100, 250))
	tmp.NumSimul[0] = int(Clamp(int32(tmp.NumSimul[0]), 2, int32(MaxSimul)))
	tmp.NumSimul[1] = int(Clamp(int32(tmp.NumSimul[1]), int32(tmp.NumSimul[0]), int32(MaxSimul)))
	tmp.NumTag[0] = int(Clamp(int32(tmp.NumTag[0]), 2, int32(MaxSimul)))
	tmp.NumTag[1] = int(Clamp(int32(tmp.NumTag[1]), int32(tmp.NumTag[0]), int32(MaxSimul)))
	tmp.PanningRange = ClampF(tmp.PanningRange, 0, 100)
	tmp.Players = int(Clamp(int32(tmp.Players), 1, int32(MaxSimul)*2))
	tmp.WavChannels = Clamp(tmp.WavChannels, 1, 256)
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
	Mp3SampleRate = int(tmp.AudioSampleRate)
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
	sys.controllerStickSensitivity = tmp.ControllerStickSensitivity
	sys.explodMax = tmp.MaxExplod
	sys.externalShaderList = tmp.ExternalShaders
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
	sys.helperMax = tmp.MaxHelper
	sys.inputButtonAssist = tmp.InputButtonAssist
	sys.inputSOCDresolution = Clamp(tmp.InputSOCDResolution, 0, 4)
	sys.lifeMul = tmp.LifeMul / 100
	sys.lifeShare = [...]bool{tmp.TeamLifeShare, tmp.TeamLifeShare}
	sys.listenPort = tmp.ListenPort
	sys.loseSimul = tmp.LoseSimul
	sys.loseTag = tmp.LoseTag
	sys.masterVolume = tmp.VolumeMaster
	sys.multisampleAntialiasing = tmp.MSAA
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
	fmt.Printf("[DEBUG][main.go][setupConfig] Assigning Joystick setting to Engine\n")
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

	return tmp
}
