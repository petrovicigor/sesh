package tui

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

// Set to true to enable debug logging to /tmp/sesh-tui-debug.log.
// Can also be enabled at runtime via SESH_DEBUG=1 environment variable.
const debugEnabled = true

var (
	debugLog    *os.File
	debugWriter *bufio.Writer
	startTime   time.Time
	debugOnce   sync.Once
	debugActive bool // resolved once at startup
)

// initDebugLog lazily opens the debug log file on first use (i.e., when TUI runs).
// Only opens if debugEnabled const is true or SESH_DEBUG=1 env var is set.
func initDebugLog() {
	debugOnce.Do(func() {
		if !debugEnabled && os.Getenv("SESH_DEBUG") != "1" {
			return
		}
		var err error
		debugLog, err = os.OpenFile("/tmp/sesh-tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			debugLog = nil
		} else {
			debugWriter = bufio.NewWriterSize(debugLog, 8192)
			debugActive = true
		}
	})
}

// resetStartTime marks the beginning of a TUI session for timing purposes.
// Must be called at the very start of Run().
func resetStartTime() {
	initDebugLog()
	startTime = time.Now()
	viewCount = 0
	if debugActive {
		fmt.Fprintf(debugWriter, "\n========== sesh tui session %s ==========\n", startTime.Format("2006-01-02 15:04:05.000"))
	}
}

// flushDebugLog flushes buffered log writes to disk and closes the file.
func flushDebugLog() {
	if debugWriter != nil {
		debugWriter.Flush()
	}
	if debugLog != nil {
		debugLog.Close()
	}
}

// logDebug logs a message with elapsed time since startup (buffered, no fsync per call).
func logDebug(format string, args ...interface{}) {
	if debugActive {
		elapsed := time.Since(startTime)
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(debugWriter, "[%6.1fms] %s\n", float64(elapsed.Microseconds())/1000.0, msg)
	}
}
