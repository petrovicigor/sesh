package tui

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

var (
	debugLog    *os.File
	debugWriter *bufio.Writer
	startTime   time.Time
)

func init() {
	var err error
	debugLog, err = os.OpenFile("/tmp/sesh-tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		debugLog = nil
	} else {
		debugWriter = bufio.NewWriterSize(debugLog, 8192)
	}
}

// resetStartTime marks the beginning of a TUI session for timing purposes.
// Must be called at the very start of Run().
func resetStartTime() {
	startTime = time.Now()
	viewCount = 0
	if debugWriter != nil {
		fmt.Fprintf(debugWriter, "\n========== sesh tui session %s ==========\n", startTime.Format("2006-01-02 15:04:05.000"))
	}
}

// flushDebugLog flushes buffered log writes to disk.
// Call at session end (after p.Run() returns).
func flushDebugLog() {
	if debugWriter != nil {
		debugWriter.Flush()
	}
}

// logTiming logs a message with elapsed time since startup (buffered, no fsync per call).
func logTiming(format string, args ...interface{}) {
	if debugWriter != nil {
		elapsed := time.Since(startTime)
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(debugWriter, "[%6.1fms] %s\n", float64(elapsed.Microseconds())/1000.0, msg)
	}
}

func logDebug(format string, args ...interface{}) {
	if debugWriter != nil {
		elapsed := time.Since(startTime)
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(debugWriter, "[%6.1fms] %s\n", float64(elapsed.Microseconds())/1000.0, msg)
	}
}
