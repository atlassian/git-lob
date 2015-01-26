package main

import (
	"bytes"
	"fmt"
	"time"
)

type ProgressCallbackType int

const (
	// Process is figuring out what to do
	ProgressCalculate ProgressCallbackType = iota
	// Process is transferring data
	ProgressTransferBytes ProgressCallbackType = iota
	// Process is skipping data because it's already up to date
	ProgressSkip ProgressCallbackType = iota
	// Process did not find the requested data, moving on
	ProgressNotFound ProgressCallbackType = iota
	// Non-fatal error
	ProgressError ProgressCallbackType = iota
)

// Collected callback data for a progress operation
type ProgressCallbackData struct {
	// What stage of the process this is for, preparing, transferring or skipping something
	Type ProgressCallbackType
	// Either a general message or an item name (e.g. file name in download stage)
	Desc string
	// If applicable, how many bytes transferred for this item
	ItemBytesDone int64
	// If applicable, how many bytes comprise this item
	ItemBytes int64
	// The number of bytes transferred for all items
	TotalBytesDone int64
	// The number of bytes needed to transfer all of this process
	TotalBytes int64
}

// Callback when progress is made during process
// return true to abort the (entire) process
type ProgressCallback func(data *ProgressCallbackData) (abort bool)

// Function to periodically (based on freq) report progress of a transfer process to the console
// callbackChan must be a channel of updates which is being populated with ProgressCallbackData
// from a goroutine at an unknown frequency. This function will then print updates every freq seconds
// of the updates received so far, collapsing duplicates (in the case of very frequent transfer updates)
// and filling in the blanks with an updated transfer rate in the case of no updates in the time.
func ReportProgressToConsole(callbackChan <-chan *ProgressCallbackData, op string, freq time.Duration) {
	// Update the console once every half second regardless of how many callbacks
	// (or zero callbacks, so we can reduce xfer rate)
	tickChan := time.Tick(freq)
	// samples of data transferred over the last 4 ticks (2s average)
	transferRate := NewTransferRateCalculator(4)

	var lastTotalBytesDone int64
	var lastTime = time.Now()
	var lastProgress *ProgressCallbackData
	complete := false
	lastConsoleLineLen := 0
	for _ = range tickChan {
		// We run this every 0.5s
		var finalDownloadProgress *ProgressCallbackData
		for stop := false; !stop && !complete; {
			select {
			case data := <-callbackChan:
				if data == nil {
					// channel was closed, we've finished
					stop = true
					complete = true
					break
				}

				// Some progress data is available
				// May get many of these and we only want to display the last one
				// unless it's general infoo or we're in verbose mode
				switch data.Type {
				case ProgressCalculate:
					finalDownloadProgress = nil
					LogConsole(data.Desc)
				case ProgressSkip:
					finalDownloadProgress = nil
					// Only print if verbose
					LogConsoleDebugf("Skipped: %v (Up to date)\n", data.Desc)
				case ProgressNotFound:
					finalDownloadProgress = nil
					LogConsolef("Not found: %v (Continuing)\n", data.Desc)
				case ProgressTransferBytes:
					// Print completion in verbose mode
					if data.ItemBytesDone == data.ItemBytes && GlobalOptions.Verbose {
						msg := fmt.Sprintf("%ved: %v 100%%", op, data.Desc)
						LogConsoleOverwrite(msg, lastConsoleLineLen)
						lastConsoleLineLen = len(msg)
						// Clear line on completion in verbose mode
						// Don't do this as \n in string above since we need to clear spaces after
						LogConsole("")
					} else {
						// Otherwise we only really want to display the last one
						finalDownloadProgress = data
					}
				}
			default:
				// No (more) progress data
				stop = true
			}
		}
		// Write progress data for this 0.5s if relevant
		// If either we have new progress data, or unfinished progress data from previous
		if finalDownloadProgress != nil || lastProgress != nil {
			var bytesPerSecond int64
			if finalDownloadProgress != nil && finalDownloadProgress.ItemBytes != 0 && finalDownloadProgress.TotalBytes != 0 {
				lastProgress = finalDownloadProgress
				bytesDoneThisTick := finalDownloadProgress.TotalBytesDone - lastTotalBytesDone
				lastTotalBytesDone = finalDownloadProgress.TotalBytesDone
				seconds := float32(time.Since(lastTime).Seconds())
				if seconds > 0 {
					bytesPerSecond = int64(float32(bytesDoneThisTick) / seconds)
				}
			} else {
				// Actually the default but lets be specific
				bytesPerSecond = 0
			}
			// Calculate transfer rate
			transferRate.AddSample(bytesPerSecond)
			avgRate := transferRate.Average()

			if lastProgress.ItemBytes != 0 || lastProgress.TotalBytes != 0 {
				buf := bytes.NewBufferString(fmt.Sprintf("%ving: ", op))
				if lastProgress.ItemBytes > 0 && GlobalOptions.Verbose {
					itemPercent := int((100 * lastProgress.ItemBytesDone) / lastProgress.ItemBytes)
					buf.WriteString(fmt.Sprintf("%v %d%%", lastProgress.Desc, itemPercent))
					if lastProgress.TotalBytes != 0 {
						buf.WriteString(" Overall: ")
					}
				}
				if lastProgress.TotalBytes > 0 {
					overallPercent := int((100 * lastProgress.TotalBytesDone) / lastProgress.TotalBytes)
					bytesRemaining := lastProgress.TotalBytes - lastProgress.TotalBytesDone
					secondsRemaining := bytesRemaining / avgRate
					timeRemaining := time.Duration(secondsRemaining) * time.Second

					buf.WriteString(fmt.Sprintf("%d%% (%v ETA %v)", overallPercent, FormatTransferRate(avgRate), timeRemaining))
				}
				msg := buf.String()
				LogConsoleOverwrite(msg, lastConsoleLineLen)
				lastConsoleLineLen = len(msg)
			}
		}

		if complete {
			break
		}

	}

}
