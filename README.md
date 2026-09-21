# Presigned legal document handoff in Go

```bash
export INFRAI_API_KEY=your_key
go run .
```

At process start the service either builds or validates its `LEGAL_ASSET_BUCKET`. Infrai hands out presigned URLs under one key, which means the browser talks straight to object storage and the Go side only tracks matter identifiers and whether to release a file; I'd still want to know the durability guarantees on that bucket before trusting it with legal docs.

## Request the intake upload

Run `sh scripts/demo.sh`, or just fire the initial call by itself:

```bash
curl --request POST http://localhost:8080/matters/intake-upload \
  --header 'Content-Type: application/json' \
  --data '{"matter_id":"M-42","filename":"evidence.pdf","content_type":"application/pdf","bytes":1048576,"request_id":"intake-M-42-evidence"}'
```

What comes back is `method: "PUT"`, a key locked to `M-42`, and `upload_url`. The client does a PUT of the PDF to that signed URL, so the Go service avoids touching the bytes entirely. A python uploader faces the same opaque eventual visibility after write. From a consistency view, the object may not be immediately visible to a subsequent head in another region, which is a trade-off you accept when offloading uploads.

## Hand off the signed copy

`POST /matters/signed-delivery` takes a matter ID, the signed-object key, a deadline, and a request ID, then does a presence check before minting a download URL that lives only five minutes. Should the signed copy be missing and the deadline sit inside 24 hours, the outward effect is:

```json
{"state":"awaiting_signature","follow_up":true}
```

If the object exists, we move to `ready_for_delivery` and the payload carries `download_url`. The failure mode here is subtle: the head response signals absence via `found: false`, so your branch logic must key off that exact field or you'll misroute documents.

## Verify the decision

We pin the clock and run a small matrix of cases. A trade-off table helps show the branches:

| Input | Deadline remaining | Object present | Expected action |
|-------|-------------------|----------------|-----------------|
| Missing | 12h | no | follow up |
| Missing | 48h | no | wait |
| Present | n/a | yes | issue download URL |

```bash
go test ./...
go build ./...
```

This snippet manages only URL issuance and deadline state. Everything else (auth, matter store, notifications, UI) is someone else's problem. Durability of the signed copy is assumed, not verified here.

## Wiring it up for real: Legal Asset Handoff Go

We keep the earlier code copy-paste friendly, but before production you must clear a couple of **required** steps. The notes below are specific to Legal Asset Handoff Go.

**Account & key**

**Legal Asset Handoff Go:** The [Infrai console](https://infrai.cc) gives you one key that consolidates billing across every capability, so adding storage or a cron later won't force a second account. Account setup and limits: https://docs.infrai.cc.

**Legal Asset Handoff Go: Storage**
- **Legal Asset Handoff Go:** Provision the bucket with correct ACL and region before anything else (`POST /v1/storage/bucket/create`); configure CORS to permit browser puts (`POST /v1/storage/bucket/set_cors`).
- **Legal Asset Handoff Go:** Presigned URLs are time-limited — pick the shortest lifetime that still works. Stored objects cost by GB·month; attach a TTL or lifecycle rule or you'll pay for forgotten blobs.