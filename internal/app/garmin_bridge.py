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
    if isinstance(value, (int, float)):
        timestamp = float(value)
        if timestamp > 100000000000:
            timestamp = timestamp / 1000
        try:
            return datetime.fromtimestamp(timestamp, tz=timezone.utc).isoformat().replace("+00:00", "Z")
        except (OverflowError, OSError, ValueError):
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
    user_profile_pk = extract_user_profile_pk(client)
    display_name = client.display_name or ""
    full_name = client.full_name or ""
    return {
        "accountId": str(user_profile_pk or display_name or full_name),
        "displayName": display_name,
        "fullName": full_name,
        "unitSystem": client.unit_system or "",
        "userProfilePk": str(user_profile_pk or ""),
    }


def extract_user_profile_pk(client):
    candidates = []
    try:
        profile = client.get_user_profile()
    except Exception:
        profile = {}
    candidates.append(first_nested_value(profile, ("userProfilePk", "userProfilePK", "userProfileId", "profileId", "profilePk", "id")))

    try:
        social_profile = client.client.connectapi("/userprofile-service/socialProfile")
    except Exception:
        social_profile = {}
    candidates.append(first_nested_value(social_profile, ("userProfilePk", "userProfilePK", "userProfileId", "profileId", "profilePk")))

    for candidate in candidates:
        if candidate not in (None, ""):
            return candidate
    return None


def first_nested_value(value, keys):
    normalized = {normalize_key(key) for key in keys}
    if isinstance(value, dict):
        for key, item in value.items():
            if normalize_key(key) in normalized and item not in (None, ""):
                return item
        for item in value.values():
            found = first_nested_value(item, keys)
            if found not in (None, ""):
                return found
    if isinstance(value, list):
        for item in value:
            found = first_nested_value(item, keys)
            if found not in (None, ""):
                return found
    return None


def first_value(item, keys):
    if not isinstance(item, dict):
        return None
    for wanted in (normalize_key(key) for key in keys):
        for key, value in item.items():
            if normalize_key(key) == wanted and value not in (None, ""):
                return value
    return None


def normalize_key(value):
    return "".join(ch for ch in str(value).lower() if ch.isalnum())


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


def parse_bool(value):
    if isinstance(value, bool):
        return value
    if value is None:
        return False
    return str(value).strip().lower() in ("true", "1", "yes", "retired", "inactive")


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


def parse_distance_m(value, key=""):
    parsed = parse_number(value)
    if parsed is None:
        return None
    normalized_key = normalize_key(key)
    if "mile" in normalized_key or normalized_key.endswith("mi"):
        return parsed * 1609.344
    if "kilometer" in normalized_key or normalized_key.endswith("km"):
        return parsed * 1000
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


def normalize_gear_response(client):
    user_profile_pk = extract_user_profile_pk(client)
    if not user_profile_pk:
        raise RuntimeError("missing Garmin user profile id")
    gear_payload = client.get_gear(str(user_profile_pk))
    defaults_payload = safe_gear_call(lambda: client.get_gear_defaults(str(user_profile_pk)), {})
    gear_items = extract_gear_items(gear_payload)
    gears = []
    for item in gear_items:
        gear_id = str(first_value(item, ("gearUUID", "gearUuid", "uuid", "gearPk", "gearId", "id")) or "").strip()
        if not gear_id:
            continue
        stats_payload = safe_gear_call(lambda gear_id=gear_id: client.get_gear_stats(gear_id), {})
        gears.append(normalize_gear(item, stats_payload, defaults_payload, gear_id))
    return {
        "userProfilePk": str(user_profile_pk),
        "gear": gears,
        "rawDefaults": defaults_payload,
    }


def safe_gear_call(fn, fallback):
    try:
        result = fn()
    except Exception:
        return fallback
    return result if result is not None else fallback


def extract_gear_items(payload):
    out = []

    def visit(value):
        if isinstance(value, dict):
            if looks_like_gear(value):
                out.append(value)
                return
            for item in value.values():
                visit(item)
        elif isinstance(value, list):
            for item in value:
                visit(item)

    visit(payload)
    return out


def looks_like_gear(item):
    if not isinstance(item, dict):
        return False
    keys = {normalize_key(key) for key in item.keys()}
    has_id = bool(keys & {"gearuuid", "gearuuid", "uuid", "gearpk", "gearid", "id"})
    has_name = bool(keys & {"gearname", "displayname", "name"})
    has_type = bool(keys & {"geartype", "geartypename", "category", "typename"})
    return has_id and (has_name or has_type)


def normalize_gear(item, stats, defaults, gear_id):
    name = str(first_value(item, ("displayName", "customMakeModel", "gearName", "name")) or "").strip()
    gear_type = str(first_value(item, ("gearTypeName", "gearType", "typeName", "category")) or "").strip()
    brand = str(first_value(item, ("brandName", "gearMakeName", "makeName", "brand")) or "").strip()
    model = str(first_value(item, ("modelName", "gearModelName", "customMakeModel", "model")) or "").strip()
    if brand.lower() in ("other", "unknown"):
        brand = ""
    if model.lower() in ("unknown", "unknown shoes", "unknown bike", "unknown gear"):
        model = str(first_value(item, ("customMakeModel",)) or "").strip()
    status = str(first_value(item, ("status", "gearStatus", "gearStatusName")) or "").strip().lower()
    retired = parse_bool(first_value(item, ("retired", "retiredFlag", "inactive"))) or "retired" in status
    total_distance = first_distance_m(stats, item, (
        "totalDistanceInMeters",
        "totalDistanceMeters",
        "totalDistanceM",
        "totalDistance",
        "distanceInMeters",
        "distance",
    ))
    max_distance = first_distance_m(item, stats, (
        "maxDistanceInMeters",
        "maximumDistanceInMeters",
        "maxDistanceMeters",
        "maxDistanceM",
        "maxDistance",
        "maximumMeters",
        "retirementDistance",
    ))
    first_used = first_time(item, stats, ("dateBegin", "firstUsedDate", "firstActivityDate", "createdDate", "createDate"))
    last_used = first_time(item, stats, ("dateEnd", "lastUsedDate", "lastActivityDate", "updatedDate", "updateDate"))
    defaults_for_gear = sorted(default_activity_types_for_gear(defaults, gear_id) | set(default_activity_types_from_item(item)))
    return {
        "id": gear_id,
        "name": name or gear_type or gear_id,
        "gearType": gear_type,
        "brand": brand,
        "model": model,
        "retired": retired,
        "totalDistanceM": total_distance,
        "maxDistanceM": max_distance,
        "firstUsedAt": first_used,
        "lastUsedAt": last_used,
        "defaultActivityTypes": defaults_for_gear,
        "raw": item,
        "statsRaw": stats if isinstance(stats, dict) else {},
    }


def first_distance_m(primary, secondary, keys):
    for source in (primary, secondary):
        if not isinstance(source, dict):
            continue
        for key in keys:
            value = first_nested_value(source, (key,))
            parsed = parse_distance_m(value, key)
            if parsed is not None:
                return parsed
    return None


def first_time(primary, secondary, keys):
    for source in (primary, secondary):
        value = first_nested_value(source, keys)
        parsed = parse_garmin_time(value)
        if parsed:
            return parsed
    return None


def default_activity_types_from_item(item):
    value = first_value(item, ("defaultActivityTypes", "activityTypes"))
    if isinstance(value, list):
        return [str(item).strip() for item in value if str(item).strip()]
    return []


def default_activity_types_for_gear(payload, gear_id):
    out = set()

    def visit(value, activity_hint=""):
        if isinstance(value, dict):
            current_activity = str(first_value(value, ("activityType", "activityTypeKey", "typeKey", "sportType")) or activity_hint).strip()
            current_gear = str(first_value(value, ("gearUUID", "gearUuid", "uuid", "gearPk", "gearId", "defaultGearUuid")) or "").strip()
            is_default = parse_bool(first_value(value, ("defaultGear", "default", "isDefault")))
            if current_gear == gear_id and (is_default or current_activity):
                out.add(current_activity or "default")
            for key, item in value.items():
                next_hint = current_activity or str(key)
                visit(item, next_hint)
        elif isinstance(value, list):
            for item in value:
                visit(item, activity_hint)

    visit(payload)
    return {item for item in out if item}


def normalize_gear_activity(item):
    if not isinstance(item, dict):
        return {"id": "", "raw": item}
    activity_id = item.get("activityId") or item.get("activityIdPk") or item.get("id")
    return {
        "id": str(activity_id or ""),
        "name": str(item.get("activityName") or item.get("name") or ""),
        "startTime": parse_garmin_time(item.get("startTimeGMT") or item.get("startTimeLocal") or item.get("startTime")),
        "raw": item,
    }


def gear_activities_response(client, gear_id, start, limit):
    start = max(int(start or 0), 0)
    limit = max(min(int(limit or 1000), 1000), 1)
    url = f"{client.garmin_connect_activities_baseurl}{gear_id}/gear?start={start}&limit={limit}"
    activities = client.connectapi(url)
    if not isinstance(activities, list):
        activities = []
    return {"activities": [normalize_gear_activity(item) for item in activities]}


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


def safe_health_call(errors, name, fn):
    try:
        return fn()
    except Exception as exc:
        errors[name] = str(exc)
        return None


def health_day_response(client, cdate):
    datetime.strptime(cdate, "%Y-%m-%d")
    errors = {}
    return {
        "date": cdate,
        "stats": safe_health_call(errors, "stats", lambda: client.get_stats(cdate)),
        "statsAndBody": safe_health_call(errors, "statsAndBody", lambda: client.get_stats_and_body(cdate)),
        "dailySteps": safe_health_call(errors, "dailySteps", lambda: client.get_daily_steps(cdate, cdate)),
        "heartRates": safe_health_call(errors, "heartRates", lambda: client.get_heart_rates(cdate)),
        "restingHeartRate": safe_health_call(errors, "restingHeartRate", lambda: client.get_rhr_day(cdate)),
        "sleep": safe_health_call(errors, "sleep", lambda: client.get_sleep_data(cdate)),
        "stress": safe_health_call(errors, "stress", lambda: client.get_stress_data(cdate)),
        "bodyBattery": safe_health_call(errors, "bodyBattery", lambda: client.get_body_battery(cdate, cdate)),
        "hrv": safe_health_call(errors, "hrv", lambda: client.get_hrv_data(cdate)),
        "bodyComposition": safe_health_call(errors, "bodyComposition", lambda: client.get_body_composition(cdate, cdate)),
        "errors": errors,
    }


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

    if action == "health-day":
        cdate = str(request.get("date") or "").strip()
        if not cdate:
            raise RuntimeError("missing date")
        print(json.dumps(health_day_response(client, cdate)))
        return

    if action == "gear":
        print(json.dumps(normalize_gear_response(client)))
        return

    if action == "gear-activities":
        gear_id = str(request.get("gearId") or "").strip()
        if not gear_id:
            raise RuntimeError("missing gearId")
        print(json.dumps(gear_activities_response(client, gear_id, request.get("start"), request.get("limit"))))
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
