package actor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMailboxOrdering(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	system := NewSystem(ctx, Options{MailboxSize: 128})
	defer system.Shutdown(context.Background())

	var mutex sync.Mutex
	received := make([]int, 0, 100)
	processed := make(chan struct{})
	id, err := system.Spawn(ActorFunc(func(_ *Context, message Message) error {
		mutex.Lock()
		received = append(received, message.(int))
		if len(received) == 100 {
			close(processed)
		}
		mutex.Unlock()
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if err := system.Send(ctx, id, i); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-processed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for actor messages")
	}
	if err := system.Stop(id); err != nil {
		t.Fatal(err)
	}
	if err := system.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(received) != 100 {
		t.Fatalf("received %d messages", len(received))
	}
	for i, value := range received {
		if value != i {
			t.Fatalf("message %d = %d", i, value)
		}
	}
}

func TestActorScheduleAndStop(t *testing.T) {
	t.Parallel()

	system := NewSystem(context.Background(), Options{MailboxSize: 8})
	defer system.Shutdown(context.Background())
	got := make(chan Message, 1)
	id, err := system.Spawn(ActorFunc(func(_ *Context, message Message) error {
		got <- message
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := system.Schedule(context.Background(), id, 20*time.Millisecond, "timer"); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-got:
		if value != "timer" {
			t.Fatalf("message = %v", value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduled actor message")
	}
	if err := system.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := system.Send(context.Background(), id, "after"); !errors.Is(err, ErrSystemStopped) {
		t.Fatalf("send after shutdown = %v", err)
	}
}
