package claude

import (
	"os"
	"runtime"
)

func fixtureMode(want os.FileMode) os.FileMode {
	if runtime.GOOS == "windows" {
		if want&0200 == 0 {
			return 0444
		}
		return 0666
	}
	return want
}
