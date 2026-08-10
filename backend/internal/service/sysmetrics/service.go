package sysmetrics

import (
	"sync"
	"time"
)

// Metrics contiene las métricas de hardware del servidor en un instante dado.
type Metrics struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float64 `json:"mem_percent"`
	MemUsedMB   uint64  `json:"mem_used_mb"`
	MemTotalMB  uint64  `json:"mem_total_mb"`
	DiskPercent float64 `json:"disk_percent"`
	DiskUsedGB  float64 `json:"disk_used_gb"`
	DiskTotalGB float64 `json:"disk_total_gb"`
	NetRxKBPS   float64 `json:"net_rx_kbps"`
	NetTxKBPS   float64 `json:"net_tx_kbps"`
}

// Sampler muestrea métricas de hardware cada 2 segundos en background.
type Sampler struct {
	mu      sync.RWMutex
	metrics Metrics
	stop    chan struct{}

	// estado previo para calcular deltas
	prevCPUTotal uint64
	prevCPUIdle  uint64
	prevNetRx    uint64
	prevNetTx    uint64
	prevTime     time.Time
}

// New crea un Sampler y toma una muestra inicial de referencia.
func New() *Sampler {
	s := &Sampler{stop: make(chan struct{})}
	s.prevCPUTotal, s.prevCPUIdle, _ = readCPUStat()
	s.prevNetRx, s.prevNetTx, _ = readNetDev()
	s.prevTime = time.Now()
	return s
}

// Start arranca la goroutine de muestreo. Llama a Stop() para detenerla.
func (s *Sampler) Start() {
	go s.run()
}

// Stop detiene la goroutine de muestreo.
func (s *Sampler) Stop() {
	close(s.stop)
}

// Get devuelve la última muestra de métricas.
func (s *Sampler) Get() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

func (s *Sampler) run() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.sample()
		}
	}
}

func (s *Sampler) sample() {
	now := time.Now()
	elapsed := now.Sub(s.prevTime).Seconds()

	// CPU (delta entre dos lecturas de /proc/stat)
	cpuTotal, cpuIdle, _ := readCPUStat()
	cpuPct := 0.0
	if elapsed > 0 {
		dTotal := float64(cpuTotal - s.prevCPUTotal)
		dIdle := float64(cpuIdle - s.prevCPUIdle)
		if dTotal > 0 {
			cpuPct = (1 - dIdle/dTotal) * 100
		}
	}

	// Red (bytes/s → KB/s)
	netRx, netTx, _ := readNetDev()
	netRxKBPS, netTxKBPS := 0.0, 0.0
	if elapsed > 0 {
		netRxKBPS = float64(netRx-s.prevNetRx) / elapsed / 1024
		netTxKBPS = float64(netTx-s.prevNetTx) / elapsed / 1024
	}

	// Memoria y disco (punto en el tiempo, no necesitan delta)
	memPct, memUsed, memTotal := readMemInfo()
	diskPct, diskUsed, diskTotal := readDiskInfo("/")

	s.mu.Lock()
	s.metrics = Metrics{
		CPUPercent:  clamp(cpuPct),
		MemPercent:  clamp(memPct),
		MemUsedMB:   memUsed,
		MemTotalMB:  memTotal,
		DiskPercent: clamp(diskPct),
		DiskUsedGB:  diskUsed,
		DiskTotalGB: diskTotal,
		NetRxKBPS:   netRxKBPS,
		NetTxKBPS:   netTxKBPS,
	}
	s.prevCPUTotal = cpuTotal
	s.prevCPUIdle = cpuIdle
	s.prevNetRx = netRx
	s.prevNetTx = netTx
	s.prevTime = now
	s.mu.Unlock()
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
