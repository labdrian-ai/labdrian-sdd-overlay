// Package sourcefs is the adapter that answers what ingestion asks of the machine that holds the local sources it
// reads (ingest.SourceReader), on the file system of the machine it runs on: today, the directory a relative path
// begins at, which is the directory the process works in. It only reads. Only the composition root names it.
package sourcefs

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ingest"
)

// Reader is the local machine as a SourceReader. The zero Reader is ready to use.
type Reader struct{}

var _ ingest.SourceReader = Reader{}

// WorkingDirectory is the absolute directory the process works in, asked at every call. A process that has lost
// its directory cannot tell where it is, and the failure is the operating system's own.
func (Reader) WorkingDirectory() (string, error) {
	return os.Getwd()
}
