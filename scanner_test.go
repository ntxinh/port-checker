package main

import (
	"testing"
)

const tcpFixture = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0
   1: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 67890 1 0000000000000000 100 0 0 10 0`

func TestParseProcNetIPv4(t *testing.T) {
	m, err := parseProcNet(tcpFixture, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	e := m[12345]
	if e == nil || e.Port != 8080 || e.Local != "127.0.0.1:8080" || e.State != "LISTEN" || e.Proto != "tcp" {
		t.Fatalf("bad entry: %+v", e)
	}
	if e2 := m[67890]; e2 == nil || e2.Port != 80 || e2.Local != "0.0.0.0:80" {
		t.Fatalf("bad entry2: %+v", e2)
	}
}

func TestParseProcNetIPv6(t *testing.T) {
	fix := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 999 1 0000000000000000 100 0 0 10 0`
	m, err := parseProcNet(fix, "tcp6")
	if err != nil {
		t.Fatal(err)
	}
	e := m[999]
	if e == nil || e.Port != 8080 || e.Local != "[::1]:8080" {
		t.Fatalf("got %+v", e)
	}
}

func TestParseProcNetSkipsBadLines(t *testing.T) {
	m, err := parseProcNet("header\nbogus\n"+tcpFixture, "tcp")
	if err != nil || len(m) != 2 {
		t.Fatalf("got %d %v", len(m), err)
	}
}
