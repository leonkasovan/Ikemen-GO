package main

import (
	"bufio"
	_ "embed" // Support for go:embed resources
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

//go:embed resources/defaultConfig.ini
var defaultConfig []byte

type AIrampProperties struct {
	Start [2]int32 `ini:"start"`
	End   [2]int32 `ini:"end"`
}

type KeysProperties struct {
	Joystick int    `ini:"Joystick"`
	Up       string `ini:"Up"`
	Down     string `ini:"Down"`
	Left     string `ini:"Left"`
	Right    string `ini:"Right"`
	A        string `ini:"A"`
	B        string `ini:"B"`
	C        string `ini:"C"`
	X        string `ini:"X"`
	Y        string `ini:"Y"`
	Z        string `ini:"Z"`
	Start    string `ini:"Start"`
	D        string `ini:"D"`
	W        string `ini:"W"`
	Menu     string `ini:"Menu"`
	GUID     string `ini:"GUID"`
}

// Motif represents the top-level config structure.
type Config struct {
	Def     string
	IniFile *ini.File
	Common  struct {
		Air     map[string][]string `ini:"map:^(?i)Air[0-9]*$" lua:"Air"`
		Cmd     map[string][]string `ini:"map:^(?i)Cmd[0-9]*$" lua:"Cmd"`
		Const   map[string][]string `ini:"map:^(?i)Const[0-9]*$" lua:"Const"`
		States  map[string][]string `ini:"map:^(?i)States[0-9]*$" lua:"States"`
		Fx      map[string][]string `ini:"map:^(?i)Fx[0-9]*$" lua:"Fx"`
		Modules map[string][]string `ini:"map:^(?i)Modules[0-9]*$" lua:"Modules"`
		Lua     map[string][]string `ini:"map:^(?i)Lua[0-9]*$" lua:"Lua"`
	} `ini:"Common"`
	Options struct {
		Difficulty    int     `ini:"Difficulty"`
		Life          float32 `ini:"Life"`
		Time          int32   `ini:"Time"`
		GameSpeed     int     `ini:"GameSpeed"`
		GameSpeedStep int     `ini:"GameSpeedStep"`
		Match         struct {
			Wins         int32 `ini:"Wins"`
			MaxDrawGames int32 `ini:"MaxDrawGames"`
		} `ini:"Match"`
		Credits       int  `ini:"Credits"`
		QuickContinue bool `ini:"QuickContinue"`
		AutoGuard     bool `ini:"AutoGuard"`
		GuardBreak    bool `ini:"GuardBreak"`
		Dizzy         bool `ini:"Dizzy"`
		RedLife       bool `ini:"RedLife"`
		Team          struct {
			Duplicates       bool    `ini:"Duplicates"`
			LifeShare        bool    `ini:"LifeShare"`
			PowerShare       bool    `ini:"PowerShare"`
			SingleVsTeamLife float32 `ini:"SingleVsTeamLife"`
		} `ini:"Team"`
		Simul struct {
			Min   int `ini:"Min"`
			Max   int `ini:"Max"`
			Match struct {
				Wins int32 `ini:"Wins"`
			} `ini:"Match"`
			LoseOnKO bool `ini:"LoseOnKO"`
		} `ini:"Simul"`
		Tag struct {
			Min   int `ini:"Min"`
			Max   int `ini:"Max"`
			Match struct {
				Wins int32 `ini:"Wins"`
			} `ini:"Match"`
			LoseOnKO    bool    `ini:"LoseOnKO"`
			TimeScaling float32 `ini:"TimeScaling"`
		} `ini:"Tag"`
		Turns struct {
			Min      int `ini:"Min"`
			Max      int `ini:"Max"`
			Recovery struct {
				Base  float32 `ini:"Base"`
				Bonus float32 `ini:"Bonus"`
			} `ini:"Recovery"`
		} `ini:"Turns"`
		Ratio struct {
			Recovery struct {
				Base  float32 `ini:"Base"`
				Bonus float32 `ini:"Bonus"`
			} `ini:"Recovery"`
			Level1 struct {
				Attack float32 `ini:"Attack"`
				Life   float32 `ini:"Life"`
			} `ini:"Level1"`
			Level2 struct {
				Attack float32 `ini:"Attack"`
				Life   float32 `ini:"Life"`
			} `ini:"Level2"`
			Level3 struct {
				Attack float32 `ini:"Attack"`
				Life   float32 `ini:"Life"`
			} `ini:"Level3"`
			Level4 struct {
				Attack float32 `ini:"Attack"`
				Life   float32 `ini:"Life"`
			} `ini:"Level4"`
		} `ini:"Ratio"`
	} `ini:"Options"`
	Config struct {
		Motif            string   `ini:"Motif"`
		Players          int      `ini:"Players"`
		Framerate        int      `ini:"Framerate"`
		Language         string   `ini:"Language"`
		AfterImageMax    int32    `ini:"AfterImageMax"`
		ExplodMax        int      `ini:"ExplodMax"`
		HelperMax        int32    `ini:"HelperMax"`
		ProjectileMax    int      `ini:"ProjectileMax"`
		PaletteMax       int      `ini:"PaletteMax"`
		TextMax          int      `ini:"TextMax"`
		ZoomActive       bool     `ini:"ZoomActive"`
		EscOpensMenu     bool     `ini:"EscOpensMenu"`
		FirstRun         bool     `ini:"FirstRun"`
		WindowTitle      string   `ini:"WindowTitle"`
		WindowIcon       []string `ini:"WindowIcon"`
		System           string   `ini:"System"`
		ScreenshotFolder string   `ini:"ScreenshotFolder"`
		TrainingChar     string   `ini:"TrainingChar"`
		TrainingStage    string   `ini:"TrainingStage"`
		GamepadMappings  string   `ini:"GamepadMappings"`
	} `ini:"Config"`
	Debug struct {
		AllowDebugMode      bool    `ini:"AllowDebugMode"`
		AllowDebugKeys      bool    `ini:"AllowDebugKeys"`
		ClipboardRows       int     `ini:"ClipboardRows"`
		ConsoleRows         int     `ini:"ConsoleRows"`
		ClsnDarken          bool    `ini:"ClsnDarken"`
		Font                string  `ini:"Font"`
		FontScale           float32 `ini:"FontScale"`
		StartStage          string  `ini:"StartStage"`
		ForceStageZoomout   float32 `ini:"ForceStageZoomout"`
		ForceStageZoomin    float32 `ini:"ForceStageZoomin"`
		KeepSpritesOnReload bool    `ini:"KeepSpritesOnReload"`
		MacOSUseCommandKey  bool    `ini:"MacOSUseCommandKey"`
	} `ini:"Debug"`
	Video struct {
		RenderMode              string   `ini:"RenderMode"`
		GameWidth               int32    `ini:"GameWidth"`
		GameHeight              int32    `ini:"GameHeight"`
		WindowWidth             int      `ini:"WindowWidth"`
		WindowHeight            int      `ini:"WindowHeight"`
		VSync                   int      `ini:"VSync"`
		Fullscreen              bool     `ini:"Fullscreen"`
		Borderless              bool     `ini:"Borderless"`
		RGBSpriteBilinearFilter bool     `ini:"RGBSpriteBilinearFilter"`
		MSAA                    int32    `ini:"MSAA"`
		WindowCentered          bool     `ini:"WindowCentered"`
		ExternalShaders         []string `ini:"ExternalShaders"`
		WindowScaleMode         bool     `ini:"WindowScaleMode"`
		FightAspectWidth        int32    `ini:"FightAspectWidth"`
		FightAspectHeight       int32    `ini:"FightAspectHeight"`
		KeepAspect              bool     `ini:"KeepAspect"`
		RendererDebugMode       bool     `ini:"RendererDebugMode"`
		EnableModel             bool     `ini:"EnableModel"`
		EnableModelShadow       bool     `ini:"EnableModelShadow"`
	} `ini:"Video"`
	Sound struct {
		SampleRate        int32   `ini:"SampleRate"`
		StereoEffects     bool    `ini:"StereoEffects"`
		PanningRange      float32 `ini:"PanningRange"`
		WavChannels       int32   `ini:"WavChannels"`
		MasterVolume      int     `ini:"MasterVolume"`
		PauseMasterVolume int     `ini:"PauseMasterVolume"`
		WavVolume         int     `ini:"WavVolume"`
		BGMVolume         int     `ini:"BGMVolume"`
		BGMRAMBuffer      bool    `ini:"BGMRAMBuffer"`
		MaxBGMVolume      int     `ini:"MaxBGMVolume"`
		AudioDucking      bool    `ini:"AudioDucking"`
	} `ini:"Sound"`
	Arcade struct {
		AI struct {
			RandomColor   bool `ini:"RandomColor"`
			SurvivalColor bool `ini:"SurvivalColor"`
			Ramping       bool `ini:"Ramping"`
		} `ini:"AI"`
		//items map[string]AIrampProperties `ini:"items"`
		Arcade struct {
			AIramp AIrampProperties `ini:"AIramp"`
		} `ini:"arcade"`
		Team struct {
			AIramp AIrampProperties `ini:"AIramp"`
		} `ini:"team"`
		Ratio struct {
			AIramp AIrampProperties `ini:"AIramp"`
		} `ini:"ratio"`
		Survival struct {
			AIramp AIrampProperties `ini:"AIramp"`
		} `ini:"survival"`
	} `ini:"Arcade"`
	Netplay struct {
		ListenPort      string             `ini:"ListenPort"`
		RollbackNetcode bool               `ini:"RollbackNetcode"`
		IP              map[string]string  `ini:"IP"`
		Rollback        RollbackProperties `ini:"Rollback"`
	} `ini:"Netplay"`
	Input struct {
		ButtonAssist               bool    `ini:"ButtonAssist"`
		SOCDResolution             int     `ini:"SOCDResolution"`
		ControllerStickSensitivity float32 `ini:"ControllerStickSensitivity"`
		XinputTriggerSensitivity   float32 `ini:"XinputTriggerSensitivity"`
	} `ini:"Input"`
	Keys     map[string]*KeysProperties `ini:"map:^(?i)Keys_P[0-9]+$" lua:"Keys"`
	Joystick map[string]*KeysProperties `ini:"map:^(?i)Joystick_P[0-9]+$" lua:"Joystick"`
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

// Pre-compile regex outside the loop for performance
var (
	reMotif      = regexp.MustCompile(`(?i)Motif\s*=\s*(\S+)`)
	reStartStage = regexp.MustCompile(`(?i)StartStage\s*=\s*(.+)$`)
	reWidth      = regexp.MustCompile(`(?i)GameWidth\s*=\s*(\d+)`)
	reHeight     = regexp.MustCompile(`(?i)GameHeight\s*=\s*(\d+)`)
)

func loadConfig(def string, is_mugen_game bool) (*Config, error) {
	options := ini.LoadOptions{
		Insensitive:             false,
		IgnoreInlineComment:     false,
		SkipUnrecognizableLines: true,
		AllowShadows:            false,
		UnparseableSections:     []string{},
	}

	// Choose default config source
	var defaultSrc interface{}
	// Using os.Stat logic or your custom FileExist
	if fp := FileExist("resources/defaultConfig.ini"); len(fp) != 0 {
		defaultSrc = fp
	} else {
		defaultSrc = defaultConfig // Assuming this is a []byte
	}

	var iniFile *ini.File
	var err error
	if fp := FileExist(def); len(fp) == 0 {
		iniFile, err = ini.LoadSources(options, defaultSrc)
	} else {
		iniFile, err = ini.LoadSources(options, defaultSrc, def)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %v", err)
	}

	var c Config
	c.Def = def
	c.initStruct()

	// Iterate through all sections
	for _, section := range iniFile.Sections() {
		sectionName := section.Name()
		if sectionName == ini.DEFAULT_SECTION {
			continue
		}

		for _, key := range section.Keys() {
			keyName := key.Name()
			for _, value := range key.ValueWithShadows() {
				// Clean key name for internal mapping
				fullKey := strings.ReplaceAll(sectionName, " ", "_") + "." + strings.ReplaceAll(keyName, " ", "_")
				keyParts := parseQueryPath(fullKey)
				if err := assignField(&c, keyParts, value); err != nil {
					fmt.Printf("Warning: Failed to assign key [%s]: %v\n", fullKey, err)
				}
			}
		}
	}

	c.IniFile = iniFile
	c.normalize()

	// Import Mugen setting
	if is_mugen_game {
		fmt.Printf("[config.go] import data/mugen.cfg\n")
		// FIX: Use os.Open instead of os.OpenRead
		file, err := os.Open("data/mugen.cfg")
		if err != nil {
			fmt.Printf("[config.go] Error loading data/mugen.cfg: %v\n", err)
		} else {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if len(line) < 1 || line[0] == ';' {
					continue
				}

				if res := reMotif.FindStringSubmatch(line); res != nil {
					path := res[1]
					if !exists(path) {
						c.Config.Motif = "data/system.def"
					} else {
						c.Config.Motif = path
					}
					c.SetValueUpdate("Config.Motif", c.Config.Motif)
				} else if res := reStartStage.FindStringSubmatch(line); res != nil {
					path := res[1]
					if !exists(path) {
						c.Debug.StartStage = ""
					} else {
						c.Debug.StartStage = path
					}
					c.SetValueUpdate("Debug.StartStage", c.Debug.StartStage)
				} else if res := reWidth.FindStringSubmatch(line); res != nil {
					// FIX: Use strconv.Atoi
					val, _ := strconv.Atoi(res[1])
					c.Video.GameWidth = int32(val)
					c.SetValueUpdate("Video.GameWidth", c.Video.GameWidth)
				} else if res := reHeight.FindStringSubmatch(line); res != nil {
					val, _ := strconv.Atoi(res[1])
					c.Video.GameHeight = int32(val)
					c.SetValueUpdate("Video.GameHeight", c.Video.GameHeight)
				}
			}
		}
	}

	// Import config.json if it exists
	if exists("save") && exists("save/config.json") && !exists("save/config.ini") {
		if err := importIkemenConfig("save/config.json", &c); err != nil {
			fmt.Printf("[config.go] Error importing config.json: %v\n", err)
		}
	}

	c.sysSet()
	c.Save(def)
	return &c, nil
}

// Initialize struct
func (c *Config) initStruct() {
	initMaps(reflect.ValueOf(c).Elem())
	//applyDefaultsToValue(reflect.ValueOf(c).Elem())
}

// Normalize values
func (c *Config) normalize() {
	c.SetValueUpdate("Config.Framerate", int(Clamp(int32(c.Config.Framerate), 1, 840)))
	c.SetValueUpdate("Config.Players", int(Clamp(int32(c.Config.Players), 1, int32(MaxSimul)*2)))
	c.SetValueUpdate("Options.GameSpeed", int(Clamp(int32(c.Options.GameSpeed), -9, 9)))
	c.SetValueUpdate("Options.GameSpeedStep", int(Clamp(int32(c.Options.GameSpeedStep), 1, 60)))
	c.SetValueUpdate("Options.Simul.Max", int(Clamp(int32(c.Options.Simul.Max), int32(c.Options.Simul.Min), int32(MaxSimul))))
	c.SetValueUpdate("Options.Simul.Min", int(Clamp(int32(c.Options.Simul.Min), 2, int32(MaxSimul))))
	c.SetValueUpdate("Options.Tag.Max", int(Clamp(int32(c.Options.Tag.Max), int32(c.Options.Tag.Min), int32(MaxSimul))))
	c.SetValueUpdate("Options.Tag.Min", int(Clamp(int32(c.Options.Tag.Min), 2, int32(MaxSimul))))

	// Options that determine allocation sizes should not be negative
	// Update: AfterImageMax no longer does, but it's good to keep it in mind
	//if c.Config.AfterImageMax < 0 {
	///	c.SetValueUpdate("Config.AfterImageMax", 0)
	//}

	path := strings.TrimSpace(c.Config.ScreenshotFolder)
	if path != "" {
		path = strings.ReplaceAll(path, "\\", "/")
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		if path != c.Config.ScreenshotFolder {
			c.SetValueUpdate("Config.ScreenshotFolder", path)
		}
	}

	switch c.Sound.SampleRate {
	case 22050, 44100, 48000:
	default:
		c.SetValueUpdate("Sound.SampleRate", 44100)
	}

	c.SetValueUpdate("Input.SOCDResolution", int(Clamp(int32(c.Input.SOCDResolution), 0, 4)))
	c.SetValueUpdate("Sound.MaxBGMVolume", int(Clamp(int32(c.Sound.MaxBGMVolume), 100, 250)))
	c.SetValueUpdate("Sound.PanningRange", ClampF(c.Sound.PanningRange, 0, 100))
	c.SetValueUpdate("Sound.PauseMasterVolume", int(Clamp(int32(c.Sound.PauseMasterVolume), 0, 100)))
	c.SetValueUpdate("Sound.WavChannels", Clamp(c.Sound.WavChannels, 1, 256))

	switch c.Video.MSAA {
	case 0, 2, 4, 6, 8, 16, 32:
	default:
		c.SetValueUpdate("Video.MSAA", 0)
	}
}

// Sets system values
func (c *Config) sysSet() {
	if _, ok := sys.cmdFlags["-width"]; ok {
		var w, _ = strconv.ParseInt(sys.cmdFlags["-width"], 10, 32)
		sys.gameWidth = int32(w)
	} else {
		sys.gameWidth = c.Video.GameWidth
	}
	if _, ok := sys.cmdFlags["-height"]; ok {
		var h, _ = strconv.ParseInt(sys.cmdFlags["-height"], 10, 32)
		sys.gameHeight = int32(h)
	} else {
		sys.gameHeight = c.Video.GameHeight
	}
	sys.msaa = c.Video.MSAA
	stoki := func(key string) int {
		return int(StringToKey(key))
	}
	Atoi := func(key string) int {
		if i, err := strconv.Atoi(key); err == nil {
			return i
		}
		return 999
	}
	for i := 1; i <= c.Config.Players; i++ {
		if kc, ok := c.Keys[fmt.Sprintf("keys_p%d", i)]; ok {
			newKeyConfig := KeyConfig{
				Joy:  kc.Joystick,
				GUID: kc.GUID,
				dU:   stoki(kc.Up),
				dD:   stoki(kc.Down),
				dL:   stoki(kc.Left),
				dR:   stoki(kc.Right),
				kA:   stoki(kc.A),
				kB:   stoki(kc.B),
				kC:   stoki(kc.C),
				kX:   stoki(kc.X),
				kY:   stoki(kc.Y),
				kZ:   stoki(kc.Z),
				kS:   stoki(kc.Start),
				kD:   stoki(kc.D),
				kW:   stoki(kc.W),
				kM:   stoki(kc.Menu),
			}
			sys.keyConfig = append(sys.keyConfig, newKeyConfig)
		} else {
			sys.keyConfig = append(sys.keyConfig, KeyConfig{Joy: -1})
		}
		if _, ok := sys.cmdFlags["-nojoy"]; !ok {
			if kc, ok := c.Joystick[fmt.Sprintf("joystick_p%d", i)]; ok {
				newKeyConfig := KeyConfig{
					Joy:  kc.Joystick,
					GUID: kc.GUID,
					dU:   Atoi(kc.Up),
					dD:   Atoi(kc.Down),
					dL:   Atoi(kc.Left),
					dR:   Atoi(kc.Right),
					kA:   Atoi(kc.A),
					kB:   Atoi(kc.B),
					kC:   Atoi(kc.C),
					kX:   Atoi(kc.X),
					kY:   Atoi(kc.Y),
					kZ:   Atoi(kc.Z),
					kS:   Atoi(kc.Start),
					kD:   Atoi(kc.D),
					kW:   Atoi(kc.W),
					kM:   Atoi(kc.Menu),
				}
				sys.joystickConfig = append(sys.joystickConfig, newKeyConfig)
			} else {
				sys.joystickConfig = append(sys.joystickConfig, KeyConfig{Joy: i - 1})
			}
		}
	}
}

// GetValue retrieves the value based on the query string.
func (c *Config) GetValue(query string) (interface{}, error) {
	return GetValue(c, query)
}

// SetValueUpdate sets the value based on the query string and updates the IniFile.
func (c *Config) SetValueUpdate(query string, value interface{}) error {
	return SetValueUpdate(c, c.IniFile, query, value)
}

// Save writes the current IniFile to disk, preserving comments and syntax.
func (c *Config) Save(file string) error {
	return SaveINI(c.IniFile, file)
}

func importIkemenConfig(jsonPath string, c *Config) error {
	// FIX: Use standard os.Open
	jsonFile, err := os.Open(jsonPath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %v", jsonPath, err)
	}
	defer jsonFile.Close()

	// Decode the JSON data into a map
	var result map[string]interface{}
	decoder := json.NewDecoder(jsonFile)
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("error decoding JSON: %v", err)
	}

	// Helper to handle string defaults with FileExist
	getString := func(key string, fallback string) string {
		if val, ok := result[key].(string); ok {
			if path := FileExist(val); path != "" {
				return path
			}
		}
		return fallback
	}

	// Motif
	c.Config.Motif = getString("Motif", "data/system.def")
	c.SetValueUpdate("Config.Motif", c.Config.Motif)
	fmt.Printf("[config.go] Import Motif=%v\n", c.Config.Motif)

	// CommonAir
	if v, ok := result["CommonAir"].(string); ok {
		c.Common.Air["air"] = []string{v}
		c.SetValueUpdate("Common.Air", c.Common.Air["air"])
		fmt.Printf("[config.go] Import Common.Air=%v\n", c.Common.Air["air"])
	}

	// CommonCmd
	if v, ok := result["CommonCmd"].(string); ok {
		c.Common.Cmd["cmd"] = []string{v}
		c.SetValueUpdate("Common.Cmd", c.Common.Cmd["cmd"])
		fmt.Printf("[config.go] Import Common.Cmd=%v\n", c.Common.Cmd["cmd"])
	}

	// CommonConst
	if v, ok := result["CommonConst"].(string); ok {
		c.Common.Const["const"] = []string{v}
		c.SetValueUpdate("Common.Const", c.Common.Const["const"])
		fmt.Printf("[config.go] Import Common.Const=%v\n", c.Common.Const["const"])
	}

	// CommonStates (Array of string)
	if vlist, ok := result["CommonStates"].([]interface{}); ok {
		states := make([]string, 0, len(vlist))
		for _, item := range vlist {
			if s, ok := item.(string); ok {
				states = append(states, s)
			}
		}
		c.Common.States["states"] = states
		c.SetValueUpdate("Common.States", c.Common.States["states"])
		fmt.Printf("[config.go] Import Common.States=%v\n", c.Common.States["states"])
	}

	// Video Dimensions
	if vfloat, ok := result["GameWidth"].(float64); ok {
		c.Video.GameWidth = int32(vfloat)
	} else {
		c.Video.GameWidth = 640
	}
	c.SetValueUpdate("Video.GameWidth", c.Video.GameWidth)

	if vfloat, ok := result["GameHeight"].(float64); ok {
		c.Video.GameHeight = int32(vfloat)
	} else {
		c.Video.GameHeight = 480
	}
	c.SetValueUpdate("Video.GameHeight", c.Video.GameHeight)

	// Debug and System
	c.Debug.Font = getString("DebugFont", "f-6x9.fnt")
	c.SetValueUpdate("Debug.Font", c.Debug.Font)

	c.Debug.StartStage = getString("StartStage", "")
	c.SetValueUpdate("Debug.StartStage", c.Debug.StartStage)

	c.Config.System = getString("System", "external/script/main.lua")
	c.SetValueUpdate("Config.System", c.Config.System)

	// WindowIcon (Array of string)
	if vlist, ok := result["WindowIcon"].([]interface{}); ok {
		icons := make([]string, 0, len(vlist))
		for _, item := range vlist {
			if s, ok := item.(string); ok {
				icons = append(icons, s)
			}
		}
		c.Config.WindowIcon = icons
		c.SetValueUpdate("Config.WindowIcon", c.Config.WindowIcon)
		fmt.Printf("[config.go] Import Config.WindowIcon=%v\n", c.Config.WindowIcon)
	}

	return nil
}
