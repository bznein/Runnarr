#!/usr/bin/env python3
"""Offline Garmin bridge for the persistent Runnarr testbed only."""

import json
import os
import sys
from datetime import date


class NotFoundException(RuntimeError):
    pass


def initial_state():
    return {
        "nextWorkoutId": 10001,
        "nextScheduleId": 20001,
        "workouts": {
            "9001": {
                "workoutId": "9001",
                "workoutName": "Coach-authored Garmin Workout",
                "description": "Created outside Runnarr",
                "sportType": {"sportTypeId": 1, "sportTypeKey": "running"},
                "workoutSegments": [],
            }
        },
        "scheduled": {
            "8001": {
                "workoutScheduleId": "8001",
                "workoutId": "9001",
                "date": date.today().isoformat(),
            }
        },
    }


def load_state(token_store):
    path = os.path.join(token_store, "testbed_garmin_state.json")
    try:
        with open(path, "r", encoding="utf-8") as handle:
            state = json.load(handle)
    except (FileNotFoundError, json.JSONDecodeError):
        state = initial_state()
        save_state(path, state)
    return path, state


def save_state(path, state):
    temporary = path + ".tmp"
    with open(temporary, "w", encoding="utf-8") as handle:
        json.dump(state, handle, sort_keys=True)
    os.replace(temporary, path)


def required(request, key):
    value = str(request.get(key) or "").strip()
    if not value:
        raise RuntimeError(f"missing {key}")
    return value


def normalized_workout(payload):
    return {
        "id": str(payload.get("workoutId") or ""),
        "name": str(payload.get("workoutName") or ""),
        "description": str(payload.get("description") or ""),
        "raw": payload,
    }


def normalized_schedule(payload):
    return {
        "id": str(payload.get("workoutScheduleId") or ""),
        "workoutId": str(payload.get("workoutId") or ""),
        "date": str(payload.get("date") or ""),
        "raw": payload,
    }


def main():
    request = json.load(sys.stdin)
    action = request.get("action")
    token_store = request.get("tokenStore") or os.environ.get("GARMINTOKENS")
    if not token_store:
        raise RuntimeError("missing tokenStore")
    os.makedirs(token_store, mode=0o700, exist_ok=True)
    path, state = load_state(token_store)

    if action == "connect":
        print(json.dumps({
            "accountId": "testbed-garmin",
            "displayName": "Offline Garmin Testbed",
            "fullName": "Offline Garmin Testbed",
            "unitSystem": "metric",
            "userProfilePk": "4242",
        }))
        return
    if action == "list":
        print(json.dumps({"activities": []}))
        return
    if action == "splits":
        print(json.dumps({"laps": []}))
        return
    if action == "activity-workout":
        print(json.dumps({"available": False, "intervals": [], "laps": [], "raw": {}, "errors": {}}))
        return
    if action == "health-day":
        print(json.dumps({"date": required(request, "date"), "errors": {}, "fixture": "testbed"}))
        return
    if action == "gear":
        print(json.dumps({"userProfilePk": "4242", "gear": [], "rawDefaults": []}))
        return
    if action == "gear-activities":
        print(json.dumps({"activities": []}))
        return
    if action == "download":
        raise RuntimeError("the offline testbed has no downloadable provider activities")
    if action == "workouts":
        workouts = [normalized_workout(item) for item in state["workouts"].values()]
        print(json.dumps({"workouts": workouts}))
        return
    if action == "workout":
        workout_id = required(request, "workoutId")
        payload = state["workouts"].get(workout_id)
        if payload is None:
            raise NotFoundException("testbed workout not found")
        print(json.dumps(normalized_workout(payload)))
        return
    if action == "upload-workout":
        payload = request.get("workout")
        if not isinstance(payload, dict):
            raise RuntimeError("missing workout")
        workout_id = str(state["nextWorkoutId"])
        state["nextWorkoutId"] += 1
        stored = dict(payload)
        stored["workoutId"] = workout_id
        state["workouts"][workout_id] = stored
        save_state(path, state)
        print(json.dumps(normalized_workout(stored)))
        return
    if action == "delete-workout":
        workout_id = required(request, "workoutId")
        if workout_id == "9001":
            raise RuntimeError("testbed guard: attempted to delete foreign workout")
        if state["workouts"].pop(workout_id, None) is None:
            raise NotFoundException("testbed workout not found")
        save_state(path, state)
        print(json.dumps({"ok": True}))
        return
    if action == "scheduled-workouts":
        scheduled = [normalized_schedule(item) for item in state["scheduled"].values()]
        print(json.dumps({"scheduled": scheduled}))
        return
    if action == "scheduled-workout":
        schedule_id = required(request, "scheduledWorkoutId")
        payload = state["scheduled"].get(schedule_id)
        if payload is None:
            raise RuntimeError("testbed scheduled workout not found")
        print(json.dumps(normalized_schedule(payload)))
        return
    if action == "schedule-workout":
        workout_id = required(request, "workoutId")
        scheduled_date = required(request, "date")
        workout = state["workouts"].get(workout_id)
        if workout is None or not str(workout.get("description") or "").startswith("runnarr:"):
            raise RuntimeError("testbed guard: attempted to schedule a foreign workout")
        schedule_id = str(state["nextScheduleId"])
        state["nextScheduleId"] += 1
        stored = {"workoutScheduleId": schedule_id, "workoutId": workout_id, "date": scheduled_date}
        state["scheduled"][schedule_id] = stored
        save_state(path, state)
        print(json.dumps(normalized_schedule(stored)))
        return
    if action == "unschedule-workout":
        schedule_id = required(request, "scheduledWorkoutId")
        if schedule_id == "8001":
            raise RuntimeError("testbed guard: attempted to unschedule foreign workout")
        state["scheduled"].pop(schedule_id, None)
        save_state(path, state)
        print(json.dumps({"ok": True}))
        return
    raise RuntimeError(f"unsupported action: {action}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": str(exc), "code": exc.__class__.__name__}))
        sys.exit(1)
