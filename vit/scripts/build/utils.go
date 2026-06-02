
package main

import (
	"fmt"
	"os"

	"vit-scripts/build/utils"
)

var (
	downloadFile       = utils.DownloadFile
	getCurrentPlatform = utils.GetCurrentPlatform
	CopyFile           = utils.CopyFile
)

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
