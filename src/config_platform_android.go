//go:build android

package main

import _ "embed"

//go:embed resources/defaultConfig_android.ini
var defaultConfigAndroid []byte

// platformConfigBytes returns platform-specific default config overrides.
// Android-specific overrides (e.g., OpenGL ES 3.2, fullscreen) are applied
// on top of the base defaultConfig.ini.
func platformConfigBytes() []byte {
	return defaultConfigAndroid
}
