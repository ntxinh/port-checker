package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var procRoot = "/proc"
var pidRe = regexp.MustCompile(`^[0-9]+$`)
var socketRe = regexp.MustCompile(`^socket:\[(\d+)\]$`)

type PortEntry struct {
	Proto   string
	Local   string
	Port    int
	PID     int
	Process string
	Cmdline string
	State   string
	inode   uint64
}

var procStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
	"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
	"0A": "LISTEN", "0B": "CLOSING",
}

func parseProcNet(content, proto string) (map[uint64]*PortEntry, error) {
	out := map[uint64]*PortEntry{}
	for i, line := range strings.Split(content, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		addr, port, err := decodeAddr(f[1], strings.HasSuffix(proto, "6"))
		if err != nil {
			continue // malformed line
		}
		inode, _ := strconv.ParseUint(f[9], 10, 64)
		if inode == 0 {
			continue
		}
		st := procStates[strings.ToUpper(f[3])]
		if st == "" {
			st = "0x" + f[3]
		}
		local := addr + ":" + strconv.Itoa(port)
		if strings.HasSuffix(proto, "6") {
			local = "[" + addr + "]:" + strconv.Itoa(port)
		}
		out[inode] = &PortEntry{
			Proto: proto,
			Local: local,
			Port:  port,
			State: st,
			inode: inode,
		}
	}
	return out, nil
}

func decodeAddr(field string, v6 bool) (string, int, error) {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("bad addr %q", field)
	}
	raw, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", 0, err
	}
	port64, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return "", 0, err
	}
	var ip net.IP
	if v6 {
		if len(raw) != 16 {
			return "", 0, fmt.Errorf("bad v6 len %d", len(raw))
		}
		ip = make(net.IP, 16)
		for w := range 4 { // reverse each 32-bit word
			for b := range 4 {
				ip[w*4+b] = raw[w*4+3-b]
			}
		}
	} else {
		if len(raw) != 4 {
			return "", 0, fmt.Errorf("bad v4 len %d", len(raw))
		}
		ip = net.IP{raw[3], raw[2], raw[1], raw[0]}
	}
	return ip.String(), int(port64), nil
}

func Scan() ([]PortEntry, error) {
	merged := map[uint64]*PortEntry{}
	for _, proto := range []string{"tcp", "tcp6", "udp", "udp6"} {
		b, err := os.ReadFile(filepath.Join(procRoot, "net", proto))
		if err != nil {
			continue // e.g. no ipv6 — fine
		}
		m, err := parseProcNet(string(b), proto)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			merged[k] = v
		}
	}
	resolvePIDs(merged)
	out := make([]PortEntry, 0, len(merged))
	for _, e := range merged {
		out = append(out, *e)
	}
	return out, nil
}

func resolvePIDs(inodes map[uint64]*PortEntry) {
	inodeToPID := map[uint64]int{}
	entries, _ := os.ReadDir(procRoot)
	for _, d := range entries {
		if !d.IsDir() || !pidRe.MatchString(d.Name()) {
			continue
		}
		pid, _ := strconv.Atoi(d.Name())
		fds, err := os.ReadDir(filepath.Join(procRoot, d.Name(), "fd"))
		if err != nil {
			continue // EPERM / exited
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(procRoot, d.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			if m := socketRe.FindStringSubmatch(link); m != nil {
				ino, _ := strconv.ParseUint(m[1], 10, 64)
				if _, seen := inodeToPID[ino]; !seen {
					inodeToPID[ino] = pid
				}
			}
		}
	}
	for ino, e := range inodes {
		pid := inodeToPID[ino]
		e.PID = pid
		e.Process = "?"
		if pid == 0 {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm")); err == nil {
			e.Process = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "cmdline")); err == nil {
			cl := strings.TrimRight(string(b), "\x00")
			e.Cmdline = strings.Join(strings.Split(cl, "\x00"), " ")
			if len(e.Cmdline) > 80 {
				e.Cmdline = e.Cmdline[:77] + "..."
			}
		}
	}
}
