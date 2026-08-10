//go:build linux

package sysmetrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func readCPUStat() (total, idle uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			break
		}
		var v [8]uint64
		for i := 0; i < 8; i++ {
			v[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
		}
		// user + nice + system + idle + iowait + irq + softirq + steal
		total = v[0] + v[1] + v[2] + v[3] + v[4] + v[5] + v[6] + v[7]
		idle = v[3] + v[4] // idle + iowait
		return
	}
	err = fmt.Errorf("cpu line not found")
	return
}

func readMemInfo() (pct float64, usedMB, totalMB uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	var totalKB, freeKB, buffersKB, cachedKB, sreclaimKB uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":     totalKB = v
		case "MemFree:":      freeKB = v
		case "Buffers:":      buffersKB = v
		case "Cached:":       cachedKB = v
		case "SReclaimable:": sreclaimKB = v
		}
	}
	avail := freeKB + buffersKB + cachedKB + sreclaimKB
	usedKB := totalKB - avail
	totalMB = totalKB / 1024
	usedMB = usedKB / 1024
	if totalKB > 0 {
		pct = float64(usedKB) / float64(totalKB) * 100
	}
	return
}

func readDiskInfo(path string) (pct, usedGB, totalGB float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	const gb = float64(1 << 30)
	totalGB = float64(total) / gb
	usedGB = float64(used) / gb
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	return
}

func readNetDev() (rx, tx uint64, err error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // saltar cabecera 1
	scanner.Scan() // saltar cabecera 2
	for scanner.Scan() {
		line := scanner.Text()
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colonIdx])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(line[colonIdx+1:])
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(fields[0], 10, 64)
		t, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += r
		tx += t
	}
	return
}
