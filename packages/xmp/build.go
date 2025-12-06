package xmp

/*
#cgo CFLAGS: -I. -DLIBXMP_STATIC -DLIBXMP_CORE_PLAYER
#include "xmp.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
*/
import "C"

import (
	_ "github.com/ikemen-engine/Ikemen-GO/packages/xmp/loaders"
)
