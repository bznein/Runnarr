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

func TestGarminBridgeWeatherKeepsCoordinatesForFallback(t *testing.T) {
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
print(json.dumps(bridge.normalize_weather({"latitude": 53.1, "longitude": -7.2, "temp": None})))
`
	command := exec.Command("python3", "-c", python)
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run Garmin bridge weather normalization: %v", err)
	}
	var weather struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	if err := json.Unmarshal(output, &weather); err != nil {
		t.Fatalf("decode normalized weather: %v", err)
	}
	if weather.Latitude != 53.1 || weather.Longitude != -7.2 {
		t.Fatalf("weather coordinates = (%v, %v)", weather.Latitude, weather.Longitude)
	}
}
