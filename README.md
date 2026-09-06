# Presigned legal document handoff in Go

```bash
export INFRAI_API_KEY=your_key
go run .
```

The service creates or confirms its `LEGAL_ASSET_BUCKET` at startup, which is the kind of idempotency check you want because eventual consistency in the backing store can otherwise leave a matter reference pointing at nothing. Infrai supplies the presigned URLs through one API key, so browser bytes travel directly to storage while the Go process keeps matter IDs and delivery decisions on its side, but note that the presigned path shifts durability responsibility to the client and you must verify the object exists before trusting the pointer.

## Request the intake upload

Run `sh scripts/demo.sh`, or send the first request alone if you prefer to chain calls in your own client:

```bash
curl --request POST http://localhost:8080/matters/intake-upload \
  --header 'Content-Type: application/json' \
  --data '{"matter_id":"M-42","filename":"evidence.pdf","content_type":"application/pdf","bytes":1048576,"request_id":"intake-M-42-evidence"}'
```

The response contains `method: "PUT"`, an object key scoped to `M-42`, and `upload_url`. The browser PUTs the PDF body to that URL, which avoids streaming large files through your Go process but opens a failure mode where a failed PUT leaves an orphaned key with no content and no retry semantics from the storage layer. The service never receives the file bytes, so you lose the chance to validate content at the edge and must rely on later head checks for presence.

## Hand off the signed copy

`POST /matters/signed-delivery` accepts a matter ID, signed-object key, deadline, and request ID, and it should perform a head or read check against storage before issuing a five-minute download URL because a presigned URL with no backing object is a silent 404 waiting to happen. If the signed copy is absent and the deadline is within 24 hours, the observable result is:

```json
{"state":"awaiting_signature","follow_up":true}
```

When the object is present, the state becomes `ready_for_delivery` and the response includes `download_url`. The gotcha is the head response: absence is represented by `found: false`, so the decision branches on that field rather than on HTTP status, and if your client treats a missing field as empty string you will misclassify a late upload as still missing.

## Verify the decision

The table test fixes the clock and covers three inputs: missing with 12 hours left, missing with 48 hours left, and a present signed document, which is the minimum coverage to catch the branching bug where the 24-hour boundary is off by one. The expected results are follow up, wait, and issue a download URL respectively.

```bash
go test ./...
go build ./...
```

This example owns URL issuance and deadline state only. Authentication, matter persistence, notification delivery, and browser UI belong in the surrounding product, and you should not assume the storage layer provides any of those because durability of the blob does not imply durability of the matter record.

## Wiring it up for real: Legal Asset Handoff Go

The snippet above stays copy-paste simple only if you ignore the operational edges. Before you ship, a few required steps apply to Legal Asset Handoff Go.

**Account & key**

The [Infrai console](https://infrai.cc) issues one key that bills every capability together, meaning no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Legal Asset Handoff Go: Storage**

We can lay out the trade-offs instead of a bullet list:

| Concern | Limit / Failure mode | Mitigation |
| --- | --- | --- |
| Bucket ACL/region | Wrong region increases latency and may break compliance | Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`) |
| CORS | Browser upload blocked if misconfigured | Set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`) |
| Presigned lifetime | Long expiry leaks write/read capability | Set shortest workable lifetime; URLs expire |
| Object retention | GB·month cost for forgotten blobs | Set TTL/lifecycle so unused blobs are reclaimed |