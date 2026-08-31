// Package shutdown translates process signals into an explicit close-only
// notification channel shared by the client components.
package shutdown

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// ErrRequested identifies operations interrupted by a shutdown notification.
var ErrRequested = errors.New("shutdown requested")

// Notifier owns the SIGTERM subscription and its watcher goroutine.
type Notifier struct {
	signals     chan os.Signal
	requested   chan struct{}
	stopWatcher chan struct{}
	watcherDone chan struct{}
}

// NewSIGTERMNotifier starts translating the first SIGTERM into channel closure.
func NewSIGTERMNotifier() *Notifier {
	notifier := &Notifier{
		signals:     make(chan os.Signal, 1),
		requested:   make(chan struct{}),
		stopWatcher: make(chan struct{}),
		watcherDone: make(chan struct{}),
	}
	signal.Notify(notifier.signals, syscall.SIGTERM)

	go func() {
		defer close(notifier.watcherDone)
		select {
		case <-notifier.signals:
			close(notifier.requested)
		case <-notifier.stopWatcher:
		}
	}()

	return notifier
}

// Done is closed when SIGTERM requests graceful termination.
func (notifier *Notifier) Done() <-chan struct{} {
	return notifier.requested
}

// Stop unregisters signal delivery and joins the watcher. It must be called
// exactly once after all consumers of Done have returned.
func (notifier *Notifier) Stop() {
	signal.Stop(notifier.signals)
	close(notifier.stopWatcher)
	<-notifier.watcherDone
}

// Requested reports channel closure without blocking the caller.
func Requested(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}
