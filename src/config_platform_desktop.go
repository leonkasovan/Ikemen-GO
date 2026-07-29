//go:build !android && !armdevice

package main

// platformConfigBytes returns platform-specific default config overrides.
// Desktop (Windows, Linux x64, macOS) uses the base defaultConfig.ini as-is.
func platformConfigBytes() []byte {
	return nil
}
