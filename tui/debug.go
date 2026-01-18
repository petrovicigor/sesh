package tui

import (
	"fmt"
	"os"
)

var debugLog *os.File

func init() {
	var err error
	debugLog, err = os.OpenFile("/tmp/sesh-tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		debugLog = nil
	}
}

func logDebug(format string, args ...interface{}) {
	if debugLog != nil {
		fmt.Fprintf(debugLog, format+"\n", args...)
		debugLog.Sync()
	}
}
