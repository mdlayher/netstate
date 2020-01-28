//+build linux

package netstate

import (
	"context"
	"fmt"
	"time"

	"github.com/jsimonetti/rtnetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sync/errgroup"
)

// deadlineNow is a sentinel value which will cause an immediate timeout to
// the rtnetlink listener.
var deadlineNow = time.Unix(0, 1)

// watch is the OS-specific portion of Watch.
func (w *Watcher) watch(ctx context.Context) error {
	c, err := rtnetlink.Dial(&netlink.Config{
		Groups: 0x1, // RTMGRP_LINK (TODO: move to x/sys).
	})
	if err != nil {
		return fmt.Errorf("netstate: watcher failed to dial route netlink: %w", err)
	}
	defer c.Close()

	// Wait for cancelation and then force any pending reads to time out.
	var eg errgroup.Group
	eg.Go(func() error {
		<-ctx.Done()

		if err := c.SetReadDeadline(deadlineNow); err != nil {
			return fmt.Errorf("netstate: failed to interrupt watcher: %w", err)
		}

		return nil
	})

	for {
		msgs, _, err := c.Receive()
		if err != nil {
			if ctx.Err() != nil {
				// Context canceled.
				return eg.Wait()
			}

			return fmt.Errorf("netstate: watcher failed to listen for route netlink messages: %w", err)
		}

		// Received messages; produce a changeSet and notify subscribers.
		w.notify(process(msgs))
	}
}

// process handles received route netlink messages and produces a changeSet
// suitable for use with the Watcher.notify method.
func process(msgs []rtnetlink.Message) changeSet {
	changes := make(changeSet)
	for _, m := range msgs {
		// TODO: also inspect other message types for addresses, routes, etc.
		switch m := m.(type) {
		case *rtnetlink.LinkMessage:
			c, ok := operStateChange(m.Attributes.OperationalState)
			if !ok {
				continue
			}

			iface := m.Attributes.Name
			changes[iface] = append(changes[iface], c)
		}
	}

	return changes
}

// operStateChange converts a rtnetlink.OperationalState to a Change value.
func operStateChange(s rtnetlink.OperationalState) (Change, bool) {
	switch s {
	case rtnetlink.OperStateUnknown:
		return LinkUnknown, true
	case rtnetlink.OperStateNotPresent:
		return LinkNotPresent, true
	case rtnetlink.OperStateDown:
		return LinkDown, true
	case rtnetlink.OperStateLowerLayerDown:
		return LinkLowerLayerDown, true
	case rtnetlink.OperStateTesting:
		return LinkTesting, true
	case rtnetlink.OperStateDormant:
		return LinkDormant, true
	case rtnetlink.OperStateUp:
		return LinkUp, true
	default:
		// Unhandled value, do nothing.
		return 0, false
	}
}
