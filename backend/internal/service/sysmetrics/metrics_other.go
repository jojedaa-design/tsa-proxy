//go:build !linux

package sysmetrics

// Stubs para plataformas no-Linux. El backend se compila y despliega en Linux;
// estos stubs evitan errores de compilación en entornos de desarrollo no-Linux.

func readCPUStat() (total, idle uint64, err error) { return }
func readMemInfo() (pct float64, usedMB, totalMB uint64) { return }
func readDiskInfo(_ string) (pct, usedGB, totalGB float64) { return }
func readNetDev() (rx, tx uint64, err error)       { return }
