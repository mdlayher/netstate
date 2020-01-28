//+build !linux

package netstate

import (
	"context"
	"fmt"
	"os"
	"runtime"
)

// watch is the OS-specific portion of Watch.
func (w *Watcher) watch(ctx context.Context) error {
	return fmt.Errorf("netstate: Watcher not implemented on %q: %w", runtime.GOOS, os.ErrNotExist)
}
