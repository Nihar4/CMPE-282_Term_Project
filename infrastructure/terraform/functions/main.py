"""
Cloud Function: file-ingest-fn
Triggered by GCS finalize event → publishes a `file_events` Pub/Sub message.

Deployed via Terraform `google_cloudfunctions2_function.file_ingest`.
Source is zipped by deploy.sh and uploaded to gs://<project>-fn-src.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any

from google.cloud import pubsub_v1

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("file-ingest-fn")

PROJECT_ID = os.environ.get("PROJECT_ID", "")
TOPIC = os.environ.get("PUBSUB_TOPIC", "enterprise-portal-file-events")

_publisher = pubsub_v1.PublisherClient()
_topic_path = _publisher.topic_path(PROJECT_ID, TOPIC)


def main(cloud_event: Any) -> None:
    """CloudEvent handler. `cloud_event.data` includes bucket / name."""
    data = cloud_event.data or {}
    payload = {
        "bucket": data.get("bucket"),
        "object_name": data.get("name"),
        "size": data.get("size"),
        "content_type": data.get("contentType"),
        "event_time": data.get("timeCreated"),
        "event_type": "file.uploaded",
    }
    log.info("publishing file_event: %s", payload)
    future = _publisher.publish(
        _topic_path,
        json.dumps(payload).encode("utf-8"),
        bucket=str(payload["bucket"]),
        object_name=str(payload["object_name"]),
    )
    future.result(timeout=30)
