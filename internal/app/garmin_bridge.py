#!/usr/bin/env python3
import base64
import json
import math
import os
import sys
from datetime import datetime, timezone

from garminconnect import Garmin


class MFARequired(Exception):
    pass


def parse_garmin_time(value):
    if not value:
        return None
    text = str(value).strip()
    for fmt in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S", "%Y-%m-%dT%H:%M:%S.%f"):
        try:
            return datetime.strptime(text, fmt).replace(tzinfo=timezone.utc).isoformat().replace("+00:00", "Z")
        except ValueError:
            pass
    try:
        parsed = datetime.fromisoformat(text.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def login(token_store, email=None, password=None, mfa_code=""):
    def prompt_mfa():
        if mfa_code:
            return mfa_code
        raise MFARequired("Garmin requires an MFA code. Enter the code and connect again.")

    client = Garmin(email, password, prompt_mfa=prompt_mfa)
    client.login(token_store)
    return client


def profile_response(client):
    display_name = client.display_name or ""
    full_name = client.full_name or ""
    return {
        "accountId": display_name or full_name,
        "displayName": display_name,
        "fullName": full_name,
        "unitSystem": client.unit_system or "",
    }


def normalize_activity(item):
    activity_type = item.get("activityType") or {}
    if not isinstance(activity_type, dict):
        activity_type = {}
    activity_id = item.get("activityId") or item.get("activityIdPk") or item.get("id")
    start_time = (
        item.get("startTimeGMT")
        or item.get("beginTimestamp")
        or item.get("startTimeLocal")
        or item.get("startTime")
    )
    return {
        "id": str(activity_id or ""),
        "name": str(item.get("activityName") or item.get("name") or ""),
        "sportType": str(activity_type.get("typeKey") or activity_type.get("typeId") or ""),
        "startTime": parse_garmin_time(start_time),
        "avgGradeAdjustedSpeed": item.get("avgGradeAdjustedSpeed"),
    }


def parse_number(value):
    if value is None:
        return None
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return None
    if not math.isfinite(parsed):
        return None
    return parsed


def parse_int(value):
    if value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def normalize_lap(item, fallback_index):
    if not isinstance(item, dict):
        return {
            "index": fallback_index,
            "avgGradeAdjustedSpeed": None,
        }

    index = fallback_index
    lap_index = parse_int(item.get("lapIndex"))
    if lap_index is not None:
        index = max(lap_index - 1, 0)
    else:
        message_index = parse_int(item.get("messageIndex"))
        if message_index is not None:
            index = message_index

    return {
        "index": index,
        "avgGradeAdjustedSpeed": parse_number(item.get("avgGradeAdjustedSpeed")),
    }


def download_bytes(client, activity_id):
    payload = client.download_activity(activity_id, Garmin.ActivityDownloadFormat.ORIGINAL)
    if isinstance(payload, bytes):
        return payload
    content = getattr(payload, "content", None)
    if isinstance(content, bytes):
        return content
    if isinstance(payload, str):
        return payload.encode("utf-8")
    raise RuntimeError("Garmin returned an unsupported download payload")


def main():
    request = json.load(sys.stdin)
    action = request.get("action")
    token_store = request.get("tokenStore") or os.environ.get("GARMINTOKENS")
    if not token_store:
        raise RuntimeError("missing tokenStore")
    os.makedirs(token_store, mode=0o700, exist_ok=True)

    if action == "connect":
        client = login(
            token_store,
            request.get("email") or "",
            request.get("password") or "",
            request.get("mfaCode") or "",
        )
        print(json.dumps(profile_response(client)))
        return

    client = login(token_store)
    if action == "list":
        start = int(request.get("start") or 0)
        limit = int(request.get("limit") or 100)
        activities = client.get_activities(start, limit)
        print(json.dumps({"activities": [normalize_activity(item) for item in activities]}))
        return

    if action == "download":
        activity_id = str(request.get("activityId") or "")
        if not activity_id:
            raise RuntimeError("missing activityId")
        content = download_bytes(client, activity_id)
        print(json.dumps({"contentBase64": base64.b64encode(content).decode("ascii")}))
        return

    if action == "splits":
        activity_id = str(request.get("activityId") or "")
        if not activity_id:
            raise RuntimeError("missing activityId")
        response = client.get_activity_splits(activity_id)
        lap_items = response.get("lapDTOs") if isinstance(response, dict) else []
        if not isinstance(lap_items, list):
            lap_items = []
        print(json.dumps({"laps": [normalize_lap(item, index) for index, item in enumerate(lap_items)]}))
        return

    raise RuntimeError(f"unsupported action: {action}")


if __name__ == "__main__":
    try:
        main()
    except MFARequired as exc:
        print(json.dumps({"error": str(exc), "code": "mfa_required"}))
        sys.exit(2)
    except Exception as exc:
        print(json.dumps({"error": str(exc), "code": exc.__class__.__name__}))
        sys.exit(1)
