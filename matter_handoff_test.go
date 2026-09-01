package main

import (
	"context"
	"testing"
	"time"
)

type storageStub struct {
	found bool
}

func (s storageStub) presignPut(context.Context, string, string, string, string, int64) (presignResult, error) {
	return presignResult{URL: "https://upload.example/signed"}, nil
}
func (s storageStub) presignGet(context.Context, string, string, string, string) (presignResult, error) {
	return presignResult{URL: "https://download.example/signed"}, nil
}
func (s storageStub) objectHead(context.Context, string, string) (headResult, error) {
	return headResult{Found: s.found}, nil
}

func TestSignedDeliveryDecision(t *testing.T) {
	now := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		found    bool
		deadline time.Time
		state    string
		followUp bool
		wantURL  bool
	}{
		{name: "missing near deadline triggers follow-up", deadline: now.Add(12 * time.Hour), state: "awaiting_signature", followUp: true},
		{name: "missing with time left waits", deadline: now.Add(48 * time.Hour), state: "awaiting_signature"},
		{name: "signed document is ready", found: true, deadline: now.Add(12 * time.Hour), state: "ready_for_delivery", wantURL: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handoff := matterHandoff{storage: storageStub{found: tt.found}, bucket: "legal", now: func() time.Time { return now }}
			got, err := handoff.signedDelivery(context.Background(), deliveryRequest{
				MatterID: "M-42", ObjectKey: "matters/M-42/signed/agreement.pdf", Deadline: tt.deadline, RequestID: "req-42",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tt.state || got.FollowUp != tt.followUp || (got.DownloadURL != "") != tt.wantURL {
				t.Fatalf("decision = %+v", got)
			}
		})
	}
}
