package util

import (
	"fmt"
)

var (
	GlobalOptions *Options = NewOptions()
	VersionMajor           = 0
	VersionMinor           = 5
	VersionPatch           = 0
)

func Version() string {
	return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
}
