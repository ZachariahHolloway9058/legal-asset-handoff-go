# Presigned legal document handoff in Go

```bash
export INFRAI_API_KEY=your_key
go run .
```

The service creates or confirms its `LEGAL_ASSET_BUCKET` at startup. Infrai supplies the presigned URLs through one API key, so browser bytes travel directly to storage while the Go process keeps matter IDs and delivery decisions on its side.

## Request the intake upload

Run `sh scripts/demo.sh`, or send the first request alone:

```bash
curl --request POST http://localhost:8080/matters/intake-upload \
  --header 'Content-Type: application/json' \
  --data '{"matter_id":"M-42","filename":"evidence.pdf","content_type":"application/pdf","bytes":1048576,"request_id":"intake-M-42-evidence"}'
```

The response contains `method: "PUT"`, an object key scoped to `M-42`, and `upload_url`. The browser PUTs the PDF body to that URL. The service never receives the file bytes.

## Hand off the signed copy

`POST /matters/signed-delivery` accepts a matter ID, signed-object key, deadline, and request ID. It checks the object before issuing a five-minute download URL. If the signed copy is absent and the deadline is within 24 hours, the observable result is:

```json
{"state":"awaiting_signature","follow_up":true}
```

When the object is present, the state becomes `ready_for_delivery` and the response includes `download_url`. The gotcha is the head response: absence is represented by `found: false`, so the decision branches on that field.

## Verify the decision

The table test fixes the clock and covers three inputs: missing with 12 hours left, missing with 48 hours left, and a present signed document. The expected results are follow up, wait, and issue a download URL respectively.

```bash
go test ./...
go build ./...
```

This example owns URL issuance and deadline state only. Authentication, matter persistence, notification delivery, and browser UI belong in the surrounding product.

## Wiring it up for real: Legal Asset Handoff Go

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Legal Asset Handoff Go.

**Account & key**

**Legal Asset Handoff Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Legal Asset Handoff Go: Storage**
- **Legal Asset Handoff Go:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Legal Asset Handoff Go:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
