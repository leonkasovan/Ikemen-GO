//go:build armdevice

package main

import _ "embed"

//go:embed resources/defaultConfig_armdevice.ini
var defaultConfigArmDevice []byte

// platformConfigBytes returns platform-specific default config overrides.
// ARM device (Linux arm64, e.g., Anbernic R36S) overrides disable 3D models
// and MSAA for performance on Mali-class GPUs.
func platformConfigBytes() []byte {
	return defaultConfigArmDevice
}
