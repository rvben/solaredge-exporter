package main

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestModbusRecoveryBackoff(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var failing atomic.Bool
	failing.Store(true)
	var connections atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				request := make([]byte, 12)
				if _, err := io.ReadFull(conn, request); err != nil || failing.Load() {
					return
				}
				response := make([]byte, 9+2*regInverterCount)
				copy(response, request[:4])
				binary.BigEndian.PutUint16(response[4:6], uint16(len(response)-6))
				response[6], response[7], response[8] = request[6], 3, 2*regInverterCount
				_, _ = conn.Write(response)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	backend := &ModbusBackend{
		address: listener.Addr().String(), deviceID: 1, timeout: time.Second,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), wasReachable: true,
	}
	expire := func() {
		backend.mu.Lock()
		backend.lastFailure = time.Now().Add(-backend.retryDelay - time.Second)
		backend.mu.Unlock()
	}
	for i, delay := range []time.Duration{15, 30, 60, 90, 90} {
		if _, err := backend.Read(); err == nil {
			t.Fatal("failed inverter read should return an error")
		}
		if backend.retryDelay != delay*time.Second {
			t.Fatalf("failure %d: retry delay = %s, want %s", i+1, backend.retryDelay, delay*time.Second)
		}
		before := connections.Load()
		data, err := backend.Read()
		if err != nil || data == nil || data.Reachable {
			t.Fatalf("during cooldown: data=%+v, err=%v", data, err)
		}
		if connections.Load() != before {
			t.Fatal("cooldown opened another connection")
		}
		expire()
	}
	failing.Store(false)
	data, err := backend.Read()
	if err != nil || data == nil || !data.Reachable {
		t.Fatalf("recovered inverter: data=%+v, err=%v", data, err)
	}
	failing.Store(true)
	if _, err := backend.Read(); err == nil {
		t.Fatal("new outage should attempt a read immediately")
	}
	if backend.retryDelay != 15*time.Second {
		t.Fatalf("new outage retained old backoff: %s", backend.retryDelay)
	}
}
