package stats

import (
	"math"
	"sort"

	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

const (
	// ViewAngleBufferSize is the number of ticks to keep in the buffer for angle calculations
	ViewAngleBufferSize = 40 // ~0.5 seconds at 64 tick rate

	// MinAngleDiffThreshold is the minimum angle difference in degrees that indicates a stopped movement
	MinAngleDiffThreshold = 0.2
)

// ViewAngleSnapshot stores a player's view angle at a specific tick
type ViewAngleSnapshot struct {
	Tick   int
	Yaw    float32
	Pitch  float32
	Killed *common.Player    // Will be non-nil if a kill occurred on this tick
	Weapon *common.Equipment // The weapon used for the kill, if any
}

// RingBuffer is a simple ring buffer for view angle snapshots
type RingBuffer struct {
	Buffer []ViewAngleSnapshot
	Index  int
	Size   int
}

// NewRingBuffer creates a new ring buffer with the specified size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		Buffer: make([]ViewAngleSnapshot, size),
		Index:  0,
		Size:   size,
	}
}

// Add adds a view angle snapshot to the ring buffer
func (rb *RingBuffer) Add(snapshot ViewAngleSnapshot) {
	rb.Buffer[rb.Index] = snapshot
	rb.Index = (rb.Index + 1) % rb.Size
}

// GetLast returns the last n entries in the buffer in reverse order (most recent first)
func (rb *RingBuffer) GetLast(n int) []ViewAngleSnapshot {
	if n > rb.Size {
		n = rb.Size
	}

	result := make([]ViewAngleSnapshot, n)
	for i := 0; i < n; i++ {
		idx := (rb.Index - i - 1 + rb.Size) % rb.Size
		result[i] = rb.Buffer[idx]
	}
	return result
}

// SnapVelocity represents a calculated snap velocity for a kill
type SnapVelocity struct {
	Killer     *common.Player
	Victim     *common.Player
	AngleDelta float64
	TimeDelta  float64
	Velocity   float64
	WeaponType common.EquipmentType
}

// SnapAngleCollector tracks player view angle movements and calculates snap velocities
type SnapAngleCollector struct {
	*BaseCollector
	viewBuffers    map[uint64]*RingBuffer
	snapVelocities map[uint64][]float64
	currentTick    int
	tickRate       float64
}

// NewSnapAngleCollector creates a new SnapAngleCollector
func NewSnapAngleCollector() *SnapAngleCollector {
	return &SnapAngleCollector{
		BaseCollector:  NewBaseCollector("Snap Angle Analysis", Category("aiming")),
		viewBuffers:    make(map[uint64]*RingBuffer),
		snapVelocities: make(map[uint64][]float64),
		currentTick:    0,
	}
}

// Setup initializes the collector with the demo parser
func (sac *SnapAngleCollector) Setup(parser demoinfocs.Parser, demoStats *DemoStats) {
	sac.tickRate = parser.TickRate()
	if sac.tickRate == 0 {
		sac.tickRate = 64.0 // Default tick rate for CS2 if we can't get it from the parser
	}

	// Register kill event handler
	parser.RegisterEventHandler(func(e events.Kill) {
		sac.processKill(e, demoStats)
	})

	// Register a debug handler for tick done events
	parser.RegisterEventHandler(func(e events.DataTablesParsed) {
	})
}

// processKill analyzes view angle changes before a kill to detect aim snapping
func (sac *SnapAngleCollector) processKill(e events.Kill, demoStats *DemoStats) {
	// Ignore kills without a killer (suicides, fall damage, etc.)
	if e.Killer == nil || e.Victim == nil {
		return
	}

	// Ignore team kills
	if e.Killer.Team == e.Victim.Team {
		return
	}

	killerID := e.Killer.SteamID64
	buffer, ok := sac.viewBuffers[killerID]
	if !ok || buffer == nil {
		return // No angle data for this player
	}

	// Get recent view angles
	recentAngles := buffer.GetLast(ViewAngleBufferSize)
	if len(recentAngles) < 5 { // Need at least a few samples
		return
	}

	// Find the "settling" point (t₀) where the aim stabilized before the kill
	var startSnapshot, endSnapshot ViewAngleSnapshot

	// The end snapshot is at the kill tick
	endSnapshot = recentAngles[0] // Most recent angle

	startTickFound := false

	// Walk backwards from the kill tick until we find where the aim "settled"
	// (angle difference from previous tick is less than threshold)
	for i := 1; i < len(recentAngles)-1; i++ {
		current := recentAngles[i]
		previous := recentAngles[i+1]

		// Calculate angle difference between these consecutive ticks
		yawDiff := float64(angleDiff(current.Yaw, previous.Yaw))
		pitchDiff := float64(angleDiff(current.Pitch, previous.Pitch))
		angleDelta := math.Sqrt(yawDiff*yawDiff + pitchDiff*pitchDiff)

		// If angle difference is small enough, we've found our starting point
		if angleDelta < MinAngleDiffThreshold {
			startSnapshot = previous
			startTickFound = true
			break
		}
	}

	// If we didn't find a settling point, use the oldest angle we have
	if !startTickFound && len(recentAngles) > 1 {
		startSnapshot = recentAngles[len(recentAngles)-1]
	}

	// Calculate deltas
	tickDelta := float64(endSnapshot.Tick - startSnapshot.Tick)
	if tickDelta <= 0 {
		tickDelta = 1.0 // Minimum tick difference to avoid division by zero
	}

	// Calculate 3D angle difference between start and end vectors
	yawDiff := float64(angleDiff(startSnapshot.Yaw, endSnapshot.Yaw))
	pitchDiff := float64(angleDiff(startSnapshot.Pitch, endSnapshot.Pitch))
	angleDelta := math.Sqrt(yawDiff*yawDiff + pitchDiff*pitchDiff)

	// Convert tick delta to milliseconds with safeguards
	timeDeltaMs := (tickDelta / math.Max(1.0, sac.tickRate)) * 1000.0

	// Calculate velocity in degrees per millisecond
	var velocity float64
	if timeDeltaMs > 0 {
		velocity = angleDelta / timeDeltaMs
	} else {
		velocity = 0
	}

	// Only store non-zero, valid velocities
	if velocity > 0 && !math.IsNaN(velocity) && !math.IsInf(velocity, 0) {
		// Store the velocity for this killer
		if _, ok := sac.snapVelocities[killerID]; !ok {
			sac.snapVelocities[killerID] = make([]float64, 0)
		}
		sac.snapVelocities[killerID] = append(sac.snapVelocities[killerID], velocity)
	}

	// Get or create player stats
	playerStats := demoStats.GetOrCreatePlayerStats(e.Killer)
	if playerStats != nil {
		// Increment kill count for this player
		playerStats.IncrementIntMetric(Category("aiming"), Key("snapped_kills"))
	}
}

// CollectFrame updates the view angle buffers for each player
func (sac *SnapAngleCollector) CollectFrame(parser demoinfocs.Parser, demoStats *DemoStats) {
	sac.currentTick = parser.CurrentFrame()
	gs := parser.GameState()

	for _, player := range gs.Participants().Playing() {
		if player == nil || player.SteamID64 == 0 {
			continue
		}

		// Get or create player view buffer
		playerID := player.SteamID64
		if _, ok := sac.viewBuffers[playerID]; !ok {
			sac.viewBuffers[playerID] = NewRingBuffer(ViewAngleBufferSize)
		}

		// Check if ViewDirection methods are available
		yaw := float32(0.0)
		pitch := float32(0.0)

		// Try to safely get view directions
		func() {
			defer func() {
				if r := recover(); r != nil {
				}
			}()

			yaw = player.ViewDirectionX()
			pitch = player.ViewDirectionY()
		}()

		// Store current view angles
		snapshot := ViewAngleSnapshot{
			Tick:  sac.currentTick,
			Yaw:   yaw,
			Pitch: pitch,
		}
		sac.viewBuffers[playerID].Add(snapshot)
	}
}

// CollectFinalStats calculates the 95th percentile snap velocities
func (sac *SnapAngleCollector) CollectFinalStats(demoStats *DemoStats) {
	// For each player with snap velocity data
	for playerID, velocities := range sac.snapVelocities {
		if len(velocities) == 0 {
			continue
		}

		// Get player stats
		var player *common.Player
		for _, p := range demoStats.Players {
			if p.Player.SteamID64 == playerID {
				player = &common.Player{
					Name:      p.Player.Name,
					SteamID64: p.Player.SteamID64,
				}
				break
			}
		}

		if player == nil {
			continue
		}

		// Sort velocities to calculate percentiles
		sort.Float64s(velocities)

		// Calculate 95th percentile
		p95Index := int(float64(len(velocities)) * 0.95)
		if p95Index >= len(velocities) {
			p95Index = len(velocities) - 1
		}
		p95Value := velocities[p95Index]

		// Calculate median as well
		medianIndex := len(velocities) / 2
		medianValue := velocities[medianIndex]

		// Calculate average
		sum := 0.0
		for _, v := range velocities {
			sum += v
		}
		avgValue := sum / float64(len(velocities))

		// Store statistics
		playerStats := demoStats.GetOrCreatePlayerStats(player)
		if playerStats == nil {
			continue
		}

		// Store snap velocity metrics
		playerStats.AddMetric(Category("aiming"), Key("p95_snap_velocity"), Metric{
			Type:        MetricFloat,
			FloatValue:  p95Value,
			Description: "95th percentile of aim snap velocity in degrees/ms",
		})

		playerStats.AddMetric(Category("aiming"), Key("median_snap_velocity"), Metric{
			Type:        MetricFloat,
			FloatValue:  medianValue,
			Description: "Median of aim snap velocity in degrees/ms",
		})

		playerStats.AddMetric(Category("aiming"), Key("avg_snap_velocity"), Metric{
			Type:        MetricFloat,
			FloatValue:  avgValue,
			Description: "Average aim snap velocity in degrees/ms",
		})

		playerStats.AddMetric(Category("aiming"), Key("snap_count"), Metric{
			Type:        MetricInteger,
			IntValue:    int64(len(velocities)),
			Description: "Number of aim snaps analyzed",
		})
	}
}

// Helper function to calculate the smallest angle difference between two angles
func angleDiff(a, b float32) float32 {
	diff := float32(math.Mod(float64(b-a+180), 360) - 180)
	if diff < -180 {
		diff += 360
	}
	return float32(math.Abs(float64(diff)))
}
