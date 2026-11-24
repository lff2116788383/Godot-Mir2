package actor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrActorNotFound = errors.New("actor not found")
	ErrActorStopped  = errors.New("actor stopped")
	ErrSystemStopped = errors.New("actor system stopped")
)

// ID uniquely identifies one actor in a System.
type ID uint64

// Message is any actor payload.
type Message any

// Actor is a serially-executed mailbox owner.
type Actor interface {
	Receive(*Context, Message) error
	Stop(*Context)
}

// ActorFunc adapts a function to Actor.
type ActorFunc func(*Context, Message) error

func (f ActorFunc) Receive(ctx *Context, msg Message) error { return f(ctx, msg) }
func (ActorFunc) Stop(*Context)                             {}

// Context is passed to actor callbacks and carries send/schedule helpers.
type Context struct {
	ID     ID
	System *System
	ctx    context.Context
}

// Done is closed when the actor or system stops.
func (c *Context) Done() <-chan struct{} { return c.ctx.Done() }
func (c *Context) Err() error            { return c.ctx.Err() }

// Send asynchronously enqueues a message to target.
func (c *Context) Send(target ID, msg Message) error {
	return c.System.Send(c.ctx, target, msg)
}

// Schedule delivers msg to this actor after delay.
func (c *Context) Schedule(delay time.Duration, msg Message) error {
	return c.System.Schedule(c.ctx, c.ID, delay, msg)
}

type envelope struct {
	from    ID
	message Message
}

type cell struct {
	id      ID
	inbox   chan envelope
	stop    chan struct{}
	done    chan struct{}
	close   sync.Once
	actor   Actor
	context *Context
}

func (c *cell) requestStop() {
	c.close.Do(func() { close(c.stop) })
}

// Options controls actor system behavior.
type Options struct {
	MailboxSize int
	Logger      *slog.Logger
}

// System owns actor lifecycles and mailboxes.
type System struct {
	ctx     context.Context
	cancel  context.CancelFunc
	options Options
	mu      sync.RWMutex
	cells   map[ID]*cell
	stopped atomic.Bool
	nextID  atomic.Uint64
	wg      sync.WaitGroup
}

func NewSystem(parent context.Context, options Options) *System {
	if options.MailboxSize <= 0 {
		options.MailboxSize = 256
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(parent)
	return &System{ctx: ctx, cancel: cancel, options: options, cells: make(map[ID]*cell)}
}

// Spawn creates and starts one serial actor.
func (s *System) Spawn(actor Actor) (ID, error) {
	if s.stopped.Load() || s.ctx.Err() != nil {
		return 0, ErrSystemStopped
	}
	id := ID(s.nextID.Add(1))
	cellCtx, cancel := context.WithCancel(s.ctx)
	c := &cell{
		id: id, inbox: make(chan envelope, s.options.MailboxSize),
		stop: make(chan struct{}), done: make(chan struct{}), actor: actor,
		context: &Context{ID: id, System: s, ctx: cellCtx},
	}
	c.context.System = s
	s.mu.Lock()
	if s.stopped.Load() {
		s.mu.Unlock()
		cancel()
		return 0, ErrSystemStopped
	}
	s.cells[id] = c
	s.wg.Add(1)
	s.mu.Unlock()

	go s.run(c, cancel)
	return id, nil
}

func (s *System) run(c *cell, cancel context.CancelFunc) {
	defer s.wg.Done()
	defer cancel()
	defer close(c.done)
	defer func() {
		s.mu.Lock()
		delete(s.cells, c.id)
		s.mu.Unlock()
	}()

	for {
		select {
		case <-s.ctx.Done():
			c.actor.Stop(c.context)
			return
		case <-c.stop:
			c.actor.Stop(c.context)
			return
		case env := <-c.inbox:
			if err := c.actor.Receive(c.context, env.message); err != nil {
				s.options.Logger.Error("actor receive failed", "actor", c.id, "from", env.from, "error", err)
				c.actor.Stop(c.context)
				return
			}
		}
	}
}

// Send enqueues a message without waiting for actor execution.
func (s *System) Send(ctx context.Context, target ID, msg Message) error {
	return s.SendFrom(ctx, 0, target, msg)
}

// SendFrom records the sender ID in the mailbox envelope.
func (s *System) SendFrom(ctx context.Context, from, target ID, msg Message) error {
	if s.stopped.Load() {
		return ErrSystemStopped
	}
	s.mu.RLock()
	c := s.cells[target]
	s.mu.RUnlock()
	if c == nil {
		return ErrActorNotFound
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return ErrSystemStopped
	case <-c.stop:
		return ErrActorStopped
	case <-c.done:
		return ErrActorStopped
	case c.inbox <- envelope{from: from, message: msg}:
		return nil
	}
}

// Schedule queues msg to target after delay.
func (s *System) Schedule(ctx context.Context, target ID, delay time.Duration, msg Message) error {
	if delay < 0 {
		delay = 0
	}
	timer := time.AfterFunc(delay, func() {
		_ = s.SendFrom(context.Background(), 0, target, msg)
	})
	go func() {
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-s.ctx.Done():
			timer.Stop()
		}
	}()
	return nil
}

// Stop requests one actor to stop. Pending mailbox entries are discarded.
func (s *System) Stop(id ID) error {
	s.mu.RLock()
	c := s.cells[id]
	s.mu.RUnlock()
	if c == nil {
		return ErrActorNotFound
	}
	c.requestStop()
	return nil
}

// Wait blocks until all spawned actors have exited or ctx is done.
func (s *System) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// Shutdown stops accepting sends and requests every actor to stop.
func (s *System) Shutdown(ctx context.Context) error {
	if s.stopped.CompareAndSwap(false, true) {
		s.mu.RLock()
		cells := make([]*cell, 0, len(s.cells))
		for _, c := range s.cells {
			cells = append(cells, c)
		}
		s.mu.RUnlock()
		for _, c := range cells {
			c.requestStop()
		}
		s.cancel()
	}
	return s.Wait(ctx)
}

// Count returns the number of live actors.
func (s *System) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cells)
}

func (s *System) String() string { return fmt.Sprintf("actor.System(actor=%d)", s.Count()) }
