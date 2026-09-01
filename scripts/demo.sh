#!/bin/sh
set -eu

curl --request POST http://localhost:8080/matters/intake-upload \
  --header 'Content-Type: application/json' \
  --data '{"matter_id":"M-42","filename":"evidence.pdf","content_type":"application/pdf","bytes":1048576,"request_id":"intake-M-42-evidence"}'

curl --request POST http://localhost:8080/matters/signed-delivery \
  --header 'Content-Type: application/json' \
  --data '{"matter_id":"M-42","object_key":"matters/M-42/signed/agreement.pdf","deadline":"2026-08-22T09:00:00Z","request_id":"delivery-M-42-agreement"}'
