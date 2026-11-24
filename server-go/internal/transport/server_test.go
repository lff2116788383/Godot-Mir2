package transport

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

func startTestServer(t *testing.T, handler Handler) (net.Listener, *Server, context.CancelFunc, <-chan struct{}, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{Handler: handler, SendQueueSize: 16})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(ctx, listener); err != nil {
			errCh <- err
		}
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("transport server did not stop")
		}
	})
	return listener, server, cancel, done, errCh
}

func ping(t *testing.T, conn net.Conn, tick uint32, responseID uint64) protocol.Packet {
	t.Helper()
	payload, err := protocol.AppendWirePayload(nil, protocol.CMPing{Tick: tick})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := protocol.AppendClientPacket(nil, protocol.CM_PING, payload, responseID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
	packet, err := protocol.NewPacketDecoder(conn).ReadServerPacket()
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func TestPingRoundTripAndGracefulShutdown(t *testing.T) {
	t.Parallel()

	listener, server, _, done, errCh := startTestServer(t, PingHandler{})
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	packet := ping(t, conn, 0x11223344, 0)
	if packet.Code != uint8(protocol.SM_PING) || packet.HasResponse {
		t.Fatalf("packet = %+v", packet)
	}
	var response protocol.SMPing
	if _, err := protocol.ReadWirePayload(&response, packet.Payload); err != nil {
		t.Fatal(err)
	}
	if response.Tick != 0x11223344 {
		t.Fatalf("tick = 0x%x", response.Tick)
	}

	packet = ping(t, conn, 7, 300)
	if !packet.HasResponse || packet.ResponseID != 300 {
		t.Fatalf("response packet = %+v", packet)
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatal(err)
			}
		default:
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

func TestMalformedPacketIsIsolated(t *testing.T) {
	t.Parallel()

	listener, _, _, _, _ := startTestServer(t, PingHandler{})
	bad, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Write([]byte{0x7f}); err != nil {
		t.Fatal(err)
	}
	_ = bad.SetReadDeadline(time.Now().Add(2 * time.Second))
	one := make([]byte, 1)
	if _, err := bad.Read(one); err == nil {
		t.Fatal("expected malformed connection to close")
	}
	_ = bad.Close()

	good, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	if packet := ping(t, good, 9, 0); packet.Code != uint8(protocol.SM_PING) {
		t.Fatalf("packet = %+v", packet)
	}
}

type lifecycleCounter struct {
	opened atomic.Int32
	closed atomic.Int32
}

func (c *lifecycleCounter) HandlePacket(ctx context.Context, session *Session, packet protocol.Packet) error {
	return PingHandler{}.HandlePacket(ctx, session, packet)
}
func (c *lifecycleCounter) SessionOpened(context.Context, *Session) { c.opened.Add(1) }
func (c *lifecycleCounter) SessionClosed(context.Context, *Session) { c.closed.Add(1) }

func TestSessionLifecycle(t *testing.T) {
	t.Parallel()

	counter := &lifecycleCounter{}
	listener, server, _, _, _ := startTestServer(t, counter)
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if packet := ping(t, conn, 1, 0); packet.Code != uint8(protocol.SM_PING) {
		t.Fatalf("packet = %+v", packet)
	}
	_ = conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if counter.closed.Load() == 1 && server.SessionCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if counter.opened.Load() != 1 || counter.closed.Load() != 1 {
		t.Fatalf("lifecycle opened=%d closed=%d", counter.opened.Load(), counter.closed.Load())
	}
}
