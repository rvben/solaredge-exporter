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

	"github.com/prometheus/client_golang/prometheus"
)

// Two HTTP scrapes must not open competing connections to the single-slot inverter.
func TestConcurrentModbusReadsAreSerialized(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
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
				defer func() { _ = conn.Close() }()
				defer active.Add(-1)
				if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
					t.Error(err)
					return
				}
				request := make([]byte, 12)
				if _, err := io.ReadFull(conn, request); err != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
				response := make([]byte, 9+2*regInverterCount)
				copy(response, request[:4])
				binary.BigEndian.PutUint16(response[4:6], uint16(len(response)-6))
				response[6], response[7], response[8] = request[6], 3, 2*regInverterCount
				if _, err := conn.Write(response); err != nil {
					t.Error(err)
				}
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
	if requests.Load() != 1 {
		t.Fatalf("concurrent upstream reads = %d, want one shared read", requests.Load())
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewCollector(backend, nil))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	foundLocal := false
	for _, family := range families {
		if family.GetName() == "solaredge_fallback_active" {
			foundLocal = len(family.Metric) == 1 && family.Metric[0].GetGauge().GetValue() == 0
		}
	}
	if !foundLocal {
		t.Fatal("Modbus-only scrape must explicitly identify local provenance")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
	server.Wait()
	if requests.Load() != 2 {
		t.Fatalf("upstream reads = %d, want one shared read plus collector scrape", requests.Load())
	}
	if peak.Load() != 1 {
		t.Fatalf("concurrent upstream connections = %d, want 1", peak.Load())
	}
}
