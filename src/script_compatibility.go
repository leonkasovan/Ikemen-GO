package main

import (
	// "fmt"
	// "math"
	// "math/rand"

	// "os"
	// "path/filepath"
	// "reflect"
	// "runtime"
	// "strconv"
	// "strings"
	// "time"
	// "unsafe"

	lua "github.com/ikemen-engine/Ikemen-GO/packages/gopher-lua"
)

// -------------------------------------------------------------------------------------------------
// Register external functions to be called from Lua scripts
func systemScriptInitCompatibility(l *lua.LState) {
	luaRegister(l, "commonStatesInsert", func(l *lua.LState) int {
		// sys.commonStates = append(sys.commonStates, strArg(l, 1))
		sys.cfg.Common.States["states"] = append(sys.cfg.Common.States["states"], strArg(l, 1))
		return 0
	})
}
