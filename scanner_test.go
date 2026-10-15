package main

import (
	"net"
	"os"
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

func TestScanResolvesPID(t *testing.T) {
	// fixture proc root built in test:
	//   <tmp>/net/tcp        (copy of tcpFixture, inode 12345)
	//   <tmp>/net/{tcp6,udp,udp6} (empty, header only)
	//   <tmp>/4321/fd/3 -> socket:[12345]  (symlink)
	//   <tmp>/4321/comm = "fakeproc\n"
	//   <tmp>/4321/cmdline = "fakeproc\x00--serve\x00"
	root := t.TempDir()
	writeFixtureProc(t, root) // helper below
	old := procRoot
	procRoot = root
	defer func() { procRoot = old }()

	got, err := Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	var e *PortEntry
	for i := range got {
		if got[i].Port == 8080 {
			e = &got[i]
		}
	}
	if e == nil || e.PID != 4321 || e.Process != "fakeproc" || e.Cmdline != "fakeproc --serve" {
		t.Fatalf("got %+v", e)
	}
	// inode 67890 has no fd -> PID 0, Process "?"
	for i := range got {
		if got[i].Port == 80 && (got[i].PID != 0 || got[i].Process != "?") {
			t.Fatalf("unresolved row wrong: %+v", got[i])
		}
	}
}

func writeFixtureProc(t *testing.T, root string) {
	t.Helper()
	netDir := root + "/net"
	mkAll(t, netDir)
	writeFile(t, netDir+"/tcp", tcpFixture)
	for _, p := range []string{"tcp6", "udp", "udp6"} {
		writeFile(t, netDir+"/"+p, "  sl  local_address rem_address   st\n")
	}
	fd := root + "/4321/fd"
	mkAll(t, fd)
	if err := os.Symlink("socket:[12345]", fd+"/3"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root+"/4321/comm", "fakeproc\n")
	writeFile(t, root+"/4321/cmdline", "fakeproc\x00--serve\x00")
}

func mkAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanLive(t *testing.T) {
	// manual: go test -run TestScanLive -v
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip()
	}
	defer l.Close()
	got, err := Scan()
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	for _, e := range got {
		if e.Port == port && e.PID == os.Getpid() {
			return
		}
	}
	t.Fatalf("own listener :%d not found in %d entries", port, len(got))
}
