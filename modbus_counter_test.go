package main

import (
	"encoding/binary"
	"io"
	"math"
	"net"
	"testing"
	"time"
)

// Replay the observed lifetime-counter drop/rebound over a real Modbus socket.
// Invalid counter readings must be omitted without losing the other telemetry.
func TestModbusRejectsCounterRegressionAndRecovers(t *testing.T) {
	checkCounterSequence(t, []uint32{0, 26754736, 26754264, 26754748, 0, 26754748, 0xFFFFFFFF, 26754264, 26754748, 267547480, 26754760}, map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 7: true, 9: true}, 4000)
}

func TestCounterBootstrapRecoversFromFirstReadingSpike(t *testing.T) {
	checkCounterSequence(t, []uint32{267547480, 26754736, 26754748}, map[int]bool{0: true, 1: true}, 4000)
}

func TestCounterGuardIsOptIn(t *testing.T) {
	checkCounterSequence(t, []uint32{0, 26754736, 26754264}, nil, 0)
}

func checkCounterSequence(t *testing.T, values []uint32, omitted map[int]bool, ratedW int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		i := 0
		for i < len(values) {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Error(err)
				_ = conn.Close()
				return
			}
			for {
				request := make([]byte, 12)
				if _, err := io.ReadFull(conn, request); err != nil {
					break
				}
				qty := int(binary.BigEndian.Uint16(request[10:12]))
				response := make([]byte, 9+2*qty)
				copy(response, request[:4])
				binary.BigEndian.PutUint16(response[4:6], uint16(len(response)-6))
				response[6], response[7], response[8] = request[6], 3, byte(2*qty)
				if binary.BigEndian.Uint16(request[8:10]) == regInverterStart {
					binary.BigEndian.PutUint32(response[9+offEnergyHi*2:], values[i])
					i++
				}
				if _, err := conn.Write(response); err != nil {
					t.Error(err)
					break
				}
			}
			_ = conn.Close()
		}
	}()
	backend, err := NewModbusBackend(listener.Addr().String(), 1, time.Second, testLogger(), ratedW)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	backend.counterNow = func() time.Time { clock = clock.Add(30 * time.Second); return clock }
	for i, value := range values {
		data, err := backend.Read()
		if omitted[i] {
			if err != nil || data == nil || !data.Reachable || !math.IsNaN(data.EnergyTotal) || math.IsNaN(data.ACPower) {
				t.Errorf("invalid counter %d must be omitted while retaining other telemetry: data=%+v error=%v", value, data, err)
			}
		} else if value == 0xFFFFFFFF {
			if err != nil || data == nil || !math.IsNaN(data.EnergyTotal) {
				t.Errorf("missing counter sentinel: data=%+v error=%v", data, err)
			}
		} else if err != nil || data == nil || math.IsNaN(data.EnergyTotal) || math.Abs(data.EnergyTotal-float64(value)) > .001 {
			t.Errorf("healthy reading %d: data=%+v error=%v", value, data, err)
		}
	}
	<-done
}
