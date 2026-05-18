package agent

import (
	"encoding/json"
	"math"
	"time"
)

// Camera offset constants for the auto-follow camera.
const (
	cameraOffsetSide  = 5.0 // blocks to the right of the agent (perpendicular to yaw)
	cameraOffsetAbove = 3.0 // blocks above the agent
)

// maxCameraKeyframes caps the number of keyframes in the generated timeline to
// keep file size and spline complexity reasonable.
const maxCameraKeyframes = 100

// timelinesFile is the top-level structure of ReplayMod's timelines.json.
// The map key is the timeline name (empty string for the default timeline).
type timelinesFile map[string][]timelineTrack

// timelineTrack represents one track (e.g. time-remap or camera path).
type timelineTrack struct {
	Keyframes     []timelineKeyframe    `json:"keyframes"`
	Segments      []int                 `json:"segments"`
	Interpolators []timelineInterpolator `json:"interpolators"`
}

// timelineKeyframe is a single keyframe in a track.
type timelineKeyframe struct {
	Time       int                `json:"time"`
	Properties map[string]any     `json:"properties"`
}

// timelineInterpolator describes how to blend between keyframes.
type timelineInterpolator struct {
	Type       any      `json:"type"`
	Properties []string `json:"properties"`
}

// catmullRomType is the interpolator type descriptor for camera paths.
type catmullRomType struct {
	Type  string  `json:"type"`
	Alpha float64 `json:"alpha"`
}

// generateTimelinesJSON builds a ReplayMod-compatible timelines.json from
// position snapshots collected during recording. It produces two tracks:
//   - A linear time-remap track (1:1 render-time to replay-timestamp)
//   - A Catmull-Rom camera path that follows the agent with an offset
//
// startTime is the recording start time used to convert snapshot CapturedAt
// values into replay-relative millisecond timestamps.
func generateTimelinesJSON(snapshots []positionSnapshot, startTime time.Time) ([]byte, error) {
	if len(snapshots) < 2 {
		return nil, nil
	}

	// Subsample if we have too many snapshots.
	keyframes := subsampleSnapshots(snapshots, maxCameraKeyframes)

	// Build the time-remap track (1:1 mapping).
	timeRemapTrack := buildTimeRemapTrack(keyframes, startTime)

	// Build the camera path track.
	cameraTrack := buildCameraTrack(keyframes, startTime)

	timelines := timelinesFile{
		"": {timeRemapTrack, cameraTrack},
	}

	return json.Marshal(timelines)
}

// buildTimeRemapTrack creates a linear time-remap track that maps render-time
// to replay-timestamp at a 1:1 ratio.
func buildTimeRemapTrack(keyframes []positionSnapshot, startTime time.Time) timelineTrack {
	trackKeyframes := make([]timelineKeyframe, len(keyframes))
	for idx, snapshot := range keyframes {
		timestampMs := int(snapshot.CapturedAt.Sub(startTime).Milliseconds())
		trackKeyframes[idx] = timelineKeyframe{
			Time:       timestampMs,
			Properties: map[string]any{"timestamp": timestampMs},
		}
	}

	segments := make([]int, max(len(keyframes)-1, 0))
	for idx := range segments {
		segments[idx] = 0
	}

	return timelineTrack{
		Keyframes: trackKeyframes,
		Segments:  segments,
		Interpolators: []timelineInterpolator{
			{
				Type:       "linear",
				Properties: []string{"timestamp"},
			},
		},
	}
}

// buildCameraTrack creates a Catmull-Rom spline camera path that follows the
// agent with a fixed offset to the right side and above.
func buildCameraTrack(keyframes []positionSnapshot, startTime time.Time) timelineTrack {
	trackKeyframes := make([]timelineKeyframe, len(keyframes))
	for idx, snapshot := range keyframes {
		timestampMs := int(snapshot.CapturedAt.Sub(startTime).Milliseconds())
		camX, camY, camZ := cameraPosition(snapshot.X, snapshot.Y, snapshot.Z, snapshot.Yaw)
		camYaw, camPitch := cameraRotation(camX, camY, camZ, snapshot.X, snapshot.Y, snapshot.Z)

		trackKeyframes[idx] = timelineKeyframe{
			Time: timestampMs,
			Properties: map[string]any{
				"camera:position": []float64{camX, camY, camZ},
				"camera:rotation": []float64{camYaw, camPitch, 0.0},
			},
		}
	}

	segments := make([]int, max(len(keyframes)-1, 0))
	for idx := range segments {
		segments[idx] = 0
	}

	return timelineTrack{
		Keyframes: trackKeyframes,
		Segments:  segments,
		Interpolators: []timelineInterpolator{
			{
				Type:       catmullRomType{Type: "catmull-rom-spline", Alpha: 0.5},
				Properties: []string{"camera:rotation", "camera:position"},
			},
		},
	}
}

// cameraPosition computes the camera world position given the agent's position
// and yaw. The camera sits to the right of the agent (perpendicular to yaw
// direction) and above it.
func cameraPosition(agentX, agentY, agentZ float64, agentYaw float64) (float64, float64, float64) {
	yawRad := agentYaw * math.Pi / 180.0
	// Perpendicular right offset: rotate yaw 90° clockwise.
	camX := agentX + math.Cos(yawRad)*cameraOffsetSide
	camZ := agentZ + math.Sin(yawRad)*cameraOffsetSide
	camY := agentY + cameraOffsetAbove
	return camX, camY, camZ
}

// cameraRotation computes the yaw and pitch (in degrees) for the camera to
// look at the target position from the camera position.
func cameraRotation(camX, camY, camZ, targetX, targetY, targetZ float64) (float64, float64) {
	deltaX := targetX - camX
	deltaY := targetY - camY
	deltaZ := targetZ - camZ
	horizontalDist := math.Sqrt(deltaX*deltaX + deltaZ*deltaZ)

	// Yaw: angle in the XZ plane (Minecraft convention: 0=south, increases clockwise)
	yaw := -math.Atan2(deltaX, deltaZ) * 180.0 / math.Pi

	// Pitch: angle from horizontal (negative = looking down)
	pitch := -math.Atan2(deltaY, horizontalDist) * 180.0 / math.Pi

	return yaw, pitch
}

// subsampleSnapshots reduces the number of snapshots to at most maxCount by
// selecting evenly-spaced entries. The first and last snapshot are always
// included.
func subsampleSnapshots(snapshots []positionSnapshot, maxCount int) []positionSnapshot {
	if len(snapshots) <= maxCount {
		return snapshots
	}

	result := make([]positionSnapshot, 0, maxCount)
	step := float64(len(snapshots)-1) / float64(maxCount-1)
	for idx := range maxCount {
		srcIdx := int(math.Round(float64(idx) * step))
		if srcIdx >= len(snapshots) {
			srcIdx = len(snapshots) - 1
		}
		result = append(result, snapshots[srcIdx])
	}
	return result
}
