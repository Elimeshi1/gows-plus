package server

import (
	"testing"
	"time"

	gowsLog "github.com/devlikeapro/gows/log"
	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types/events"
)

func newTestServer() *Server {
	return &Server{
		log:       gowsLog.Stdout("test", "ERROR", false),
		listeners: map[string]map[uuid.UUID]*eventListener{},
	}
}

func isPoisoned(listener *eventListener) bool {
	select {
	case <-listener.poisoned:
		return true
	default:
		return false
	}
}

func TestSendEventToAllListenersPoisonsStuckListener(t *testing.T) {
	s := newTestServer()
	listener := s.addListener("session", uuid.New(), nil)

	// Nobody reads the events, so we fill the buffer and then drop
	for i := 0; i < eventListenerBuffer+maxConsecutiveDrops; i++ {
		s.SendEventToAllListeners("session", i)
	}

	if !isPoisoned(listener) {
		t.Fatal("expected the listener to be poisoned after maxConsecutiveDrops")
	}
}

func TestSendEventToAllListenersResetsDropsOnDelivery(t *testing.T) {
	s := newTestServer()
	listener := s.addListener("session", uuid.New(), nil)

	// Fill the buffer, then alternate: drop a few, read one, drop a few...
	for i := 0; i < eventListenerBuffer; i++ {
		s.SendEventToAllListeners("session", i)
	}
	for round := 0; round < 10; round++ {
		for i := 0; i < maxConsecutiveDrops-1; i++ {
			s.SendEventToAllListeners("session", i)
		}
		<-listener.events
		s.SendEventToAllListeners("session", "delivered")
	}

	if isPoisoned(listener) {
		t.Fatal("a listener that keeps up must not be poisoned")
	}
}

func TestSendEventToAllListenersDoesNotBlockOnPoisonedListener(t *testing.T) {
	s := newTestServer()
	listener := s.addListener("session", uuid.New(), nil)
	listener.poison()

	done := make(chan struct{})
	go func() {
		s.SendEventToAllListeners("session", "event")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SendEventToAllListeners blocked on a poisoned listener")
	}
}

func TestSendEventToAllListenersSkipsExcludedTypes(t *testing.T) {
	s := newTestServer()
	exclude := map[string]struct{}{"events.HistorySync": {}}
	listener := s.addListener("session", uuid.New(), exclude)

	// The excluded type must not take a buffer slot: those events are the bulky
	// ones, and holding them would push out events the listener actually wants.
	s.SendEventToAllListeners("session", &events.HistorySync{})
	s.SendEventToAllListeners("session", &events.Receipt{})

	if len(listener.events) != 1 {
		t.Fatalf("expected only the wanted event to be buffered, got %d", len(listener.events))
	}
	if _, ok := (<-listener.events).(*events.Receipt); !ok {
		t.Fatal("expected the buffered event to be the receipt")
	}
}
