package simulator

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/clusterops/backend/internal/models"
)

// gpuTelemetry produces realistic GPU metric snapshots for every GPU on every
// node. Utilization follows a phase-shifted sine wave with noise, modulated
// by whether the GPU is allocated. Memory and temperature track utilization.
//
// Curve shape per GPU:
//   idle:     util  2–8%,  mem  1–4 GB,  temp 33–40°C,  power  65–80 W
//   ramp:     util  10–60% (first 30s of a job)
//   running:  util  60–98%,  mem 40–75 GB, temp 65–85°C, power 280–400 W
//   throttle: util  30–50% (thermal event), temp 87–92°C
func (s *Simulator) emitGPUMetrics() []models.GPUMetric {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	now := time.Now()
	var metrics []models.GPUMetric

	for _, node := range s.state.nodes {
		for g := 0; g < node.GPUCount; g++ {
			key := fmt.Sprintf("%s:%d", node.ID, g)

			// Advance the phase counter for this GPU.
			s.state.gpuPhase[key] += 0.08 // ~2π every ~78 ticks (6.5 min)

			phase := s.state.gpuPhase[key]
			allocated := g < node.AllocatedGPUs

			// --- utilization ---
			var util float64
			if node.Status == models.NodeStatusUnavailable {
				util = 0
			} else if allocated {
				// Sine wave 60–95% + ±5% noise
				base := 77.5 + 17.5*math.Sin(phase)
				util = clamp(base+randNoise(5), 55, 99)
			} else {
				// Idle: very low utilization
				util = clamp(3+randNoise(4), 0, 10)
			}

			// --- memory ---
			var memUsed float64
			if allocated {
				// 50–75 GB for active training (attention KV cache fills up)
				memUsed = clamp(62+10*math.Sin(phase+0.5)+randNoise(5), 40, 79)
			} else {
				memUsed = clamp(1.5+randNoise(1), 0.5, 4)
			}

			// --- temperature --- tracks utilization with lag
			var temp float64
			if allocated {
				temp = clamp(55+util*0.3+randNoise(3), 50, 92)
			} else {
				temp = clamp(36+randNoise(3), 30, 45)
			}

			// --- power ---
			var power float64
			if allocated {
				power = clamp(80+util*3.3+randNoise(15), 70, 420)
			} else {
				power = clamp(72+randNoise(8), 60, 90)
			}

			m := models.GPUMetric{
				NodeID:         node.ID,
				GPUIndex:       g,
				Timestamp:      now,
				UtilizationPct: util,
				MemoryUsedGB:   memUsed,
				MemoryTotalGB:  node.GPUMemoryTotalGB[g],
				TemperatureC:   temp,
				PowerWatts:     power,
			}
			metrics = append(metrics, m)

			// Update in-place so the node's current snapshot reflects latest values.
			node.GPUUtilization[g] = util
			node.GPUMemoryUsedGB[g] = memUsed
			node.GPUTemperature[g] = temp
			node.GPUPowerWatts[g] = power
		}
		node.LastSeen = now
	}

	return metrics
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// randNoise returns a random value in [-amplitude, +amplitude].
func randNoise(amplitude float64) float64 {
	return (rand.Float64()*2 - 1) * amplitude
}
