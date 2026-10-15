package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

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
