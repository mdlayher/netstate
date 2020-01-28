//+build linux

package netstate_test

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/mdlayher/netstate"
	"golang.org/x/sys/unix"
)

func TestIntegrationWatcherWatch(t *testing.T) {
	dummy, done := dummyInterface(t)
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	// Register interest in various link state changes which will be triggered
	// on the dummy interface.
	w := netstate.NewWatcher()
	watchC := w.Subscribe(dummy, netstate.LinkAny)

	// Start the watcher and ensure the goroutine is scheduled before the main
	// goroutine continues.
	semaC := make(chan struct{})
	go func() {
		defer wg.Done()
		close(semaC)

		if err := w.Watch(ctx); err != nil {
			panicf("failed to watch: %v", err)
		}
	}()

	<-semaC

	// Trigger interface state changes and ensure events are received for
	// those changes.
	var changes []netstate.Change

	for i := 0; i < 3; i++ {
		// Alternate bringing the interface up and down.
		dir := "up"
		if i == 1 {
			dir = "down"
		}

		shell(t, "ip", "link", "set", dir, dummy)
		changes = append(changes, <-watchC)
	}

	// Now that the changes have been received, stop the Watcher.
	cancel()
	wg.Wait()

	// It's difficult for this test to be performed in a deterministic way, so
	// just check for expected event types caused as a result of bringing the
	// interface up and down.
	//
	// Interestingly, dummy interfaces appear to be set to state "unknown" when
	// brought up, so check for that.
	if len(changes) == 0 {
		t.Fatal("no link changes were detected")
	}

	for _, c := range changes {
		switch c {
		case netstate.LinkDown, netstate.LinkUnknown:
			// Expected change.
			t.Logf("change: %q", c)
		default:
			t.Fatalf("unexpected link change: %q", c)
		}
	}
}

func dummyInterface(t *testing.T) (string, func()) {
	t.Helper()
	skipUnprivileged(t)

	var (
		r     = rand.New(rand.NewSource(time.Now().UnixNano()))
		dummy = fmt.Sprintf("lsdummy%d", r.Intn(65535))
	)

	// Set up a dummy interface that can be used to trigger state change
	// notifications.
	// TODO: use rtnetlink.
	shell(t, "ip", "link", "add", dummy, "type", "dummy")

	done := func() {
		// Clean up the interface.
		shell(t, "ip", "link", "del", dummy)
	}

	return dummy, done
}

func skipUnprivileged(t *testing.T) {
	const ifName = "lsprobe0"
	shell(t, "ip", "tuntap", "add", ifName, "mode", "tun")
	shell(t, "ip", "link", "del", ifName)
}

func shell(t *testing.T, name string, arg ...string) {
	t.Helper()

	bin, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("failed to look up binary path: %v", err)
	}

	t.Logf("$ %s %v", bin, arg)

	cmd := exec.Command(bin, arg...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command %q: %v", name, err)
	}

	if err := cmd.Wait(); err != nil {
		// Shell operations in these tests require elevated privileges.
		if cmd.ProcessState.ExitCode() == int(unix.EPERM) {
			t.Skipf("skipping, permission denied: %v", err)
		}

		t.Fatalf("failed to wait for command %q: %v", name, err)
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
