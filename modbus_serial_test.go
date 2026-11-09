package main

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Two HTTP scrapes must not open competing connections to the single-slot inverter.
func TestConcurrentModbusReadsAreSerialized(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var active, peak, requests atomic.Int32
	var server sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			requests.Add(1)
			n := active.Add(1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			server.Add(1)
			go func() {
				defer server.Done()
				defer conn.Close()
				defer active.Add(-1)
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				request := make([]byte, 12)
				if _, err := io.ReadFull(conn, request); err != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
				response := make([]byte, 9+2*regInverterCount)
				copy(response, request[:4])
				binary.BigEndian.PutUint16(response[4:6], uint16(len(response)-6))
				response[6], response[7], response[8] = request[6], 3, 2*regInverterCount
				conn.Write(response)
			}()
		}
	}()
	backend := &ModbusBackend{address: listener.Addr().String(), deviceID: 1, timeout: time.Second, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), wasReachable: true}
	var clients sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		clients.Add(1)
		go func() {
			defer clients.Done()
			<-start
			if _, err := backend.Read(); err != nil {
				t.Errorf("read: %v", err)
			}
		}()
	}
	close(start)
	clients.Wait()
	listener.Close()
	<-done
	server.Wait()
	if requests.Load() != 1 {
		t.Fatalf("upstream reads = %d, want one shared read", requests.Load())
	}
	if peak.Load() != 1 {
		t.Fatalf("concurrent upstream connections = %d, want 1", peak.Load())
	}
}
