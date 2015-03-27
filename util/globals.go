package util

import (
	"fmt"
)

var (
	GlobalOptions  *Options = NewOptions()
	VersionMajor            = 0
	VersionMinor            = 4
	VersionPatch            = 0
	VersionBuildID string   // populated in build.sh to the git hash
)

func Version() string {
	if VersionBuildID != "" {
		return fmt.Sprintf("%d.%d.%d [%v]", VersionMajor, VersionMinor, VersionPatch, VersionBuildID)
	} else {
		return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
	}
}
