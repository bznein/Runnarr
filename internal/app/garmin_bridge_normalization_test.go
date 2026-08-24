package app

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"testing"
)

func TestGarminBridgeIntervalPaceMatchesGarminAverageSpeed(t *testing.T) {
	python := `
import importlib.util
import json
import sys
import types

garminconnect = types.ModuleType("garminconnect")
garminconnect.Garmin = object
sys.modules["garminconnect"] = garminconnect
spec = importlib.util.spec_from_file_location("garmin_bridge", "garmin_bridge.py")
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)
interval = bridge.normalize_interval({
    "type": "INTERVAL_ACTIVE",
    "averageSpeed": 5.09499979019165,
    "averageMovingSpeed": 5.181694935944121,
}, 0, {}, {})
print(json.dumps(interval))
`
	command := exec.Command("python3", "-c", python)
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run Garmin bridge normalization: %v", err)
	}
	var interval struct {
		AvgPaceSPKM float64 `json:"avgPaceSPKM"`
	}
	if err := json.Unmarshal(output, &interval); err != nil {
		t.Fatalf("decode normalized interval: %v", err)
	}
	want := 1000 / 5.09499979019165
	if math.Abs(interval.AvgPaceSPKM-want) > 0.0001 {
		t.Fatalf("normalized pace = %.6f, want Garmin average-speed pace %.6f", interval.AvgPaceSPKM, want)
	}
}
