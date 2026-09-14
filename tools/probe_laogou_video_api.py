#!/usr/bin/env python3
"""Probe the Laogou async video API while allowing at most one create attempt."""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
import re
import sys
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import requests
from urllib3.exceptions import HTTPError as Urllib3HTTPError
from urllib.parse import urlsplit, urlunsplit


DEFAULT_BASE_URL = "https://api.laogou.org"
DEFAULT_OUTPUT_DIR = Path("outputs/laogou_video_probe")
CONTENT_PROBE_BYTES = 4096
TRANSIENT_STATUSES = {429, 502, 503, 504}
TERMINAL_SUCCESS = {"succeeded", "success", "completed", "done", "finished"}
TERMINAL_FAILURE = {"failed", "error", "failure", "expired", "cancelled", "canceled"}
SAFE_RESPONSE_HEADERS = {
    "content-type",
    "content-length",
    "content-range",
    "content-encoding",
    "accept-ranges",
    "content-disposition",
    "retry-after",
    "location",
    "x-request-id",
    "request-id",
    "date",
}


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def read_api_key(source: Path) -> str:
    """Read a plain UTF-8 file containing only the key; never load Python code."""
    value = source.read_text(encoding="utf-8-sig").strip()
    if not value or any(char.isspace() for char in value):
        raise ValueError("Key file must contain a single non-empty API key")
    return value


def redact_url(raw: str) -> str:
    try:
        parsed = urlsplit(raw)
    except ValueError:
        return raw
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return raw
    # Signed query strings often grant direct access to the generated asset.
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, "[REDACTED]" if parsed.query else "", ""))


def sanitize(value: Any, key: str = "") -> Any:
    key_lower = key.lower()
    if any(marker in key_lower for marker in ("authorization", "api_key", "token", "secret")):
        return "[REDACTED]"
    if isinstance(value, dict):
        return {str(k): sanitize(v, str(k)) for k, v in value.items()}
    if isinstance(value, list):
        return [sanitize(item, key) for item in value]
    if isinstance(value, str) and value.startswith("data:"):
        return f"[REDACTED_DATA_URL:{len(value)} chars]"
    if isinstance(value, str) and ("url" in key_lower or value.startswith(("http://", "https://"))):
        return redact_url(value)
    return value


def describe_shape(value: Any) -> Any:
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "boolean"
    if isinstance(value, int):
        return "integer"
    if isinstance(value, float):
        return "number"
    if isinstance(value, str):
        return "string"
    if isinstance(value, list):
        samples = []
        for item in value[:3]:
            shape = describe_shape(item)
            if shape not in samples:
                samples.append(shape)
        return {"type": "array", "length": len(value), "item_shapes": samples}
    if isinstance(value, dict):
        return {"type": "object", "fields": {str(k): describe_shape(v) for k, v in value.items()}}
    return type(value).__name__


def safe_headers(headers: Any) -> dict[str, str]:
    result: dict[str, str] = {}
    for name, value in headers.items():
        if name.lower() in SAFE_RESPONSE_HEADERS:
            result[name] = redact_url(value) if name.lower() == "location" else value
    return result


def decode_body(body: bytes, content_type: str) -> tuple[Any, Any]:
    if "json" in content_type.lower() or body.lstrip().startswith((b"{", b"[")):
        try:
            parsed = json.loads(body.decode("utf-8"))
            sanitized = sanitize(parsed)
            return sanitized, describe_shape(sanitized)
        except (UnicodeDecodeError, json.JSONDecodeError):
            pass
    text = body.decode("utf-8", errors="replace")
    return text[:4000], "string"


def perform_request(
    method: str,
    base_url: str,
    path: str,
    api_key: str,
    *,
    payload: dict[str, Any] | None = None,
    idempotency_key: str | None = None,
    timeout: int = 60,
    max_body_bytes: int = 1024 * 1024,
    range_probe: bool = False,
) -> dict[str, Any]:
    url = base_url.rstrip("/") + path
    # Some upstream Cloudflare rules reject urllib's default client signature.
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Accept": "application/json",
        "User-Agent": requests.utils.default_user_agent(),
    }
    body = None
    if payload is not None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if idempotency_key:
        headers["Idempotency-Key"] = idempotency_key
    if range_probe:
        headers["Accept"] = "*/*"
        headers["Accept-Encoding"] = "identity"
        headers["Range"] = f"bytes=0-{max_body_bytes - 1}"

    started = time.monotonic()
    response = None
    try:
        # Use requests' standard transport and client signature. This is
        # important because urllib and requests expose different client/TLS
        # signatures to upstream WAFs.
        response = requests.request(
            method,
            url,
            headers=headers,
            json=payload if payload is not None else None,
            timeout=timeout,
            stream=range_probe,
            allow_redirects=False,
        )
        status = response.status_code
        response_headers = safe_headers(response.headers)
        if range_probe:
            # Read one extra byte so an oversized response cannot pass merely
            # because the probe truncated it to the requested range length.
            response_body = response.raw.read(max_body_bytes + 1)
        else:
            response_body = response.content[:max_body_bytes]
    except (requests.exceptions.RequestException, Urllib3HTTPError) as exc:
        return {
            "requested_at": utc_now(),
            "method": method,
            "path": path,
            "network_error": f"{type(exc).__name__}: {exc}",
            "elapsed_ms": round((time.monotonic() - started) * 1000),
        }
    finally:
        if response is not None:
            response.close()

    content_type = response_headers.get("Content-Type", response_headers.get("content-type", ""))
    result: dict[str, Any] = {
        "requested_at": utc_now(),
        "method": method,
        "path": path,
        "status_code": status,
        "elapsed_ms": round((time.monotonic() - started) * 1000),
        "headers": response_headers,
    }
    if range_probe and not ("json" in content_type.lower()):
        result["body"] = {
            "kind": "binary_probe",
            "bytes_read": len(response_body),
            "first_32_bytes_hex": response_body[:32].hex(),
        }
        result["body_shape"] = {"type": "binary", "bytes_read": "integer", "first_32_bytes_hex": "string"}
    else:
        result["body"], result["body_shape"] = decode_body(response_body, content_type)
    return result


def load_json(path: Path, default: Any) -> Any:
    if not path.exists():
        return default
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temp_path = path.with_suffix(path.suffix + ".tmp")
    temp_path.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")
    os.replace(temp_path, path)


def parsed_body(result: dict[str, Any]) -> dict[str, Any]:
    body = result.get("body")
    return body if isinstance(body, dict) else {}


def has_api_error(result: dict[str, Any]) -> bool:
    body = parsed_body(result)
    envelopes = [body]
    if isinstance(body.get("data"), dict):
        envelopes.append(body["data"])
    return any(
        envelope.get("error") not in (None, False, "", {}, [])
        or envelope.get("success") is False
        or str(envelope.get("status", "")).strip().lower() in TERMINAL_FAILURE
        for envelope in envelopes
    )


def normalized_headers(result: dict[str, Any]) -> dict[str, str]:
    return {str(k).lower(): str(v).strip() for k, v in result.get("headers", {}).items()}


def models_probe_error(result: dict[str, Any]) -> str:
    if result.get("status_code") != 200 or has_api_error(result):
        return "Models request failed"
    media_type = normalized_headers(result).get("content-type", "").split(";", 1)[0].strip().lower()
    if media_type != "application/json" and not (media_type.startswith("application/") and media_type.endswith("+json")):
        return "Models response must have a JSON Content-Type"
    # Match the supplier's documented models envelope, without hardcoding
    # model names or capabilities that can change upstream.
    models = parsed_body(result).get("models")
    if not isinstance(models, list) or not models or not all(
        isinstance(model, dict) and isinstance(model.get("id"), str) and model["id"].strip()
        for model in models
    ):
        return "Models response must contain a non-empty models array with model IDs"
    return ""


def content_probe_error(result: dict[str, Any]) -> str:
    headers = normalized_headers(result)
    if result.get("status_code") != 206:
        return "Content request must return HTTP 206"
    if headers.get("content-type", "").split(";", 1)[0].strip().lower() != "video/mp4":
        return "Content response must be video/mp4"
    if headers.get("content-encoding", "identity").lower() != "identity":
        return "Content response must use identity encoding for byte comparison"
    match = re.fullmatch(r"bytes ([0-9]+)-([0-9]+)/([0-9]+|\*)", headers.get("content-range", ""), re.IGNORECASE)
    if match is None:
        return "Invalid Content-Range"
    start, end = int(match[1]), int(match[2])
    if start != 0 or end < start or end >= CONTENT_PROBE_BYTES:
        return "Content-Range lies outside the requested probe range"
    if match[3] != "*" and int(match[3]) <= end:
        return "Content-Range total must exceed its end offset"
    expected_bytes = end - start + 1
    if parsed_body(result).get("bytes_read") != expected_bytes:
        return "Downloaded byte count does not match Content-Range"
    if "content-length" in headers:
        length = headers["content-length"]
        if re.fullmatch(r"[0-9]+", length) is None or int(length) != expected_bytes:
            return "Content-Length does not match Content-Range"
    return ""


def task_id_from(result: dict[str, Any]) -> str:
    body = parsed_body(result)
    for key in ("task_id", "id"):
        value = body.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    data = body.get("data")
    if isinstance(data, dict):
        for key in ("task_id", "id"):
            value = data.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip()
    return ""


def task_status_from(result: dict[str, Any]) -> str:
    body = parsed_body(result)
    value = body.get("status")
    if not isinstance(value, str) and isinstance(body.get("data"), dict):
        value = body["data"].get("status")
    return value.strip().lower() if isinstance(value, str) else ""


def retry_delay(result: dict[str, Any], fallback: int) -> int:
    headers = result.get("headers", {})
    raw = headers.get("Retry-After", headers.get("retry-after", "")) if isinstance(headers, dict) else ""
    try:
        return max(1, min(int(raw), 60))
    except (TypeError, ValueError):
        return fallback


def is_nonretryable_access_block(result: dict[str, Any]) -> bool:
    """Cloudflare/browser-signature blocks require supplier action, not retries."""
    body = parsed_body(result)
    return (
        result.get("status_code") == 403
        and (
            body.get("cloudflare_error") is True
            or body.get("error_name") == "browser_signature_banned"
            or body.get("owner_action_required") is True
        )
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--create", action="store_true", help="Allow the single paid create attempt, then poll it")
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL)
    parser.add_argument("--key-file", type=Path, help="UTF-8 file containing only the API key; otherwise use LAOGOU_API_KEY")
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    parser.add_argument("--image", type=Path, help="Local reference image; sent as a data URL in the images array")
    parser.add_argument("--prompt", default="A matchstick figure walking on a clean white background, cinematic motion")
    parser.add_argument("--model", default="seedance2.5")
    parser.add_argument("--duration", type=int, default=30)
    parser.add_argument("--ratio", default="16:9")
    parser.add_argument("--resolution", default="720p")
    parser.add_argument("--poll-interval", type=int, default=10)
    parser.add_argument("--max-wait", type=int, default=900)
    args = parser.parse_args()

    output_dir: Path = args.output_dir.resolve()
    state_path = output_dir / "state.json"
    report_path = output_dir / "report.json"
    state = load_json(state_path, {})
    report = load_json(report_path, {"started_at": utc_now(), "calls": []})
    try:
        api_key = read_api_key(args.key_file) if args.key_file else os.environ.get("LAOGOU_API_KEY", "").strip()
    except (OSError, ValueError):
        parser.error("Cannot read --key-file; provide a UTF-8 file containing only the API key")
    if not api_key or any(char.isspace() for char in api_key):
        parser.error("Set LAOGOU_API_KEY or pass --key-file with a single API key")

    models_result = perform_request("GET", args.base_url, "/v1/media/models", api_key, timeout=30)
    models_error = models_probe_error(models_result)
    if models_error:
        models_result["validation_error"] = models_error
    report["calls"].append({"name": "models", **models_result})
    write_json(report_path, report)
    print(f"models: {models_result.get('status_code', models_result.get('network_error'))}")

    if is_nonretryable_access_block(models_result):
        print("Supplier/Cloudflare blocked this client signature; stopping before any paid create request.", file=sys.stderr)
        report["stopped_reason"] = "nonretryable_access_block"
        report["finished_at"] = utc_now()
        write_json(report_path, report)
        return 6

    if not args.create:
        print(f"Read-only probe complete. Report: {report_path}")
        return 0 if not models_error else 1

    if models_error:
        report["stopped_reason"] = "models_probe_failed"
        report["finished_at"] = utc_now()
        write_json(report_path, report)
        print("Models probe failed; stopping before any paid create request.", file=sys.stderr)
        return 1

    task_id = task_id_from({"body": {"task_id": state.get("upstream_task_id")}})
    if not task_id:
        # A supplier idempotency header is not a guarantee that replay is safe.
        # Treat partial/legacy state as evidence of a possible paid submission.
        create_marker = output_dir / "create_attempt.json"
        if create_marker.exists() or any(state.get(key) for key in (
            "create_attempted", "create_attempted_at", "idempotency_key",
            "create_phase", "last_create_result", "last_create_attempted_at",
        )):
            print("The previous create result is uncertain. Do not resend POST; reconcile with the supplier and save the confirmed upstream_task_id in state.json before resuming. Keep the state and create_attempt.json files.", file=sys.stderr)
            return 3
        payload = state.get("request")
        if not isinstance(payload, dict):
            payload = {
                "model": args.model,
                "prompt": args.prompt,
                "duration": args.duration,
                "ratio": args.ratio,
                "resolution": args.resolution,
            }
            if args.image:
                image_path = args.image.resolve()
                image_bytes = image_path.read_bytes()
                if len(image_bytes) > 12 * 1024 * 1024:
                    raise RuntimeError("reference image exceeds the supplier's 12 MiB per-image limit")
                mime_type = mimetypes.guess_type(image_path.name)[0] or "application/octet-stream"
                payload["images"] = [
                    f"data:{mime_type};base64,{base64.b64encode(image_bytes).decode('ascii')}"
                ]
        idempotency_key = str(uuid.uuid4())
        state.update({
            "create_attempted": True,
            "create_attempted_at": utc_now(),
            "idempotency_key": idempotency_key,
            "request": payload,
            "create_phase": "request_locked",
        })
        # Exclusive creation also protects against two processes that read the
        # empty state simultaneously. Never remove this marker automatically,
        # including after crashes or network/HTTP errors.
        try:
            with create_marker.open("x", encoding="utf-8") as marker:
                json.dump({"idempotency_key": idempotency_key, "created_at": utc_now()}, marker)
                marker.flush()
                os.fsync(marker.fileno())
        except FileExistsError:
            print("A create attempt is already recorded; refusing another POST.", file=sys.stderr)
            return 3
        write_json(state_path, state)

        create_result = perform_request(
            "POST",
            args.base_url,
            "/v1/media/videos",
            api_key,
            payload=payload,
            idempotency_key=idempotency_key,
            timeout=60,
        )
        report["calls"].append({"name": "create", **create_result})
        write_json(report_path, report)
        print(f"create: {create_result.get('status_code', create_result.get('network_error'))}")
        create_http_ok = 200 <= create_result.get("status_code", 0) < 300
        task_id = task_id_from(create_result) if create_http_ok and not has_api_error(create_result) else ""
        if not task_id:
            if is_nonretryable_access_block(create_result):
                state["create_phase"] = "blocked_nonretryable_access"
                state["last_create_result"] = sanitize(create_result)
                state["last_create_attempted_at"] = utc_now()
                write_json(state_path, state)
                print("Cloudflare/browser-signature block detected. Do not retry; ask the supplier to allow this client or provide a permitted endpoint.", file=sys.stderr)
                return 6
            state["create_phase"] = "ambiguous_no_task_id"
            state["last_create_result"] = sanitize(create_result)
            state["last_create_attempted_at"] = utc_now()
            write_json(state_path, state)
            print("Create was not confirmed by a successful response with a task ID. Do not resend POST or delete the state/create_attempt.json files; reconcile with the supplier and save the confirmed upstream_task_id before resuming.", file=sys.stderr)
            return 3
        # Save the upstream ID immediately after the response. All later work is
        # resumable from this file even if polling times out or is interrupted.
        state["upstream_task_id"] = task_id
        state["create_phase"] = "task_created"
        state["create_status_code"] = create_result.get("status_code")
        state["task_id_saved_at"] = utc_now()
        write_json(state_path, state)
    else:
        print("Using the task ID already stored in state.json; no create request sent.")

    deadline = time.monotonic() + args.max_wait
    poll_count = 0
    status = ""
    try:
        while time.monotonic() < deadline:
            poll_count += 1
            status_result = perform_request(
                "GET",
                args.base_url,
                f"/v1/media/videos/{task_id}",
                api_key,
                timeout=30,
            )
            http_status = status_result.get("status_code")
            status = task_status_from(status_result) if http_status == 200 else ""
            report["calls"].append({"name": "status", "poll_number": poll_count, **status_result})
            report["last_status"] = status
            state["last_status"] = status
            state["last_status_checked_at"] = utc_now()
            state["last_status_response"] = sanitize(status_result)
            write_json(report_path, report)
            write_json(state_path, state)
            print(f"poll {poll_count}: http={status_result.get('status_code')} status={status or 'unknown'}")

            if http_status == 200 and has_api_error(status_result):
                report["finished_at"] = utc_now()
                write_json(report_path, report)
                return 4
            if status in TERMINAL_SUCCESS:
                break

            delay = retry_delay(status_result, args.poll_interval)
            if http_status in TRANSIENT_STATUSES or http_status == 200 or "network_error" in status_result:
                time.sleep(delay)
                continue
            report["stopped_reason"] = "status_probe_failed"
            report["finished_at"] = utc_now()
            write_json(report_path, report)
            return 6 if is_nonretryable_access_block(status_result) else 1
    except KeyboardInterrupt:
        state["interrupted_at"] = utc_now()
        state["last_status"] = status
        write_json(state_path, state)
        report["interrupted_at"] = utc_now()
        write_json(report_path, report)
        print(f"Interrupted safely. Resume with --create; task ID remains {task_id}.", file=sys.stderr)
        return 130

    if status not in TERMINAL_SUCCESS:
        report["finished_at"] = utc_now()
        report["timed_out"] = True
        state["poll_timed_out_at"] = utc_now()
        state["last_status"] = status
        write_json(state_path, state)
        write_json(report_path, report)
        print(f"Polling timed out safely. Resume with --create; task ID remains {task_id}.", file=sys.stderr)
        return 5

    content_result = perform_request(
        "GET",
        args.base_url,
        f"/v1/media/videos/{task_id}/content",
        api_key,
        timeout=120,
        max_body_bytes=CONTENT_PROBE_BYTES,
        range_probe=True,
    )
    content_error = content_probe_error(content_result)
    if content_error:
        content_result["validation_error"] = content_error
    report["calls"].append({"name": "content", **content_result})
    report["finished_at"] = utc_now()
    state["content_checked_at"] = utc_now()
    state["content_status_code"] = content_result.get("status_code")
    content_ok = not content_error
    state["phase"] = "completed" if content_ok else "content_probe_failed"
    write_json(state_path, state)
    write_json(report_path, report)
    print(f"content: {content_result.get('status_code', content_result.get('network_error'))}")
    print(f"Probe complete. Report: {report_path}")
    return 0 if content_ok else 7


if __name__ == "__main__":
    raise SystemExit(main())
