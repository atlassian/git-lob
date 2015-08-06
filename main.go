package main

import (
	"os"

	"github.com/atlassian/git-lob/cmd"
)

func main() {
	// Need to send the result code to the OS but also need to support 'defer'
	// os.Exit would finish before any defers, so wrap everything in mainImpl()
	os.Exit(cmd.MainImpl())
}
