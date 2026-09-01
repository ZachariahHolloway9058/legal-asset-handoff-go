package main

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"
)

type legalStorage interface {
	presignPut(context.Context, string, string, string, string, int64) (presignResult, error)
	presignGet(context.Context, string, string, string, string) (presignResult, error)
	objectHead(context.Context, string, string) (headResult, error)
}

type matterHandoff struct {
	storage legalStorage
	bucket  string
	now     func() time.Time
}

type intakeRequest struct {
	MatterID    string `json:"matter_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	RequestID   string `json:"request_id"`
}

type uploadInstruction struct {
	MatterID  string `json:"matter_id"`
	ObjectKey string `json:"object_key"`
	Method    string `json:"method"`
	UploadURL string `json:"upload_url"`
}

func (h *matterHandoff) startIntake(ctx context.Context, in intakeRequest) (uploadInstruction, error) {
	if in.MatterID == "" || in.RequestID == "" || in.Bytes <= 0 || in.Bytes > 25<<20 {
		return uploadInstruction{}, fmt.Errorf("matter_id, request_id, and bytes up to 25 MiB are required")
	}
	name := path.Base(in.Filename)
	if name == "." || name == "/" || strings.Contains(name, "\\") {
		return uploadInstruction{}, fmt.Errorf("filename is invalid")
	}
	key := "matters/" + in.MatterID + "/intake/" + name
	signed, err := h.storage.presignPut(ctx, h.bucket, key, in.ContentType, in.RequestID, in.Bytes)
	if err != nil {
		return uploadInstruction{}, err
	}
	return uploadInstruction{MatterID: in.MatterID, ObjectKey: key, Method: "PUT", UploadURL: signed.URL}, nil
}

type deliveryRequest struct {
	MatterID  string    `json:"matter_id"`
	ObjectKey string    `json:"object_key"`
	Deadline  time.Time `json:"deadline"`
	RequestID string    `json:"request_id"`
}

type deliveryDecision struct {
	State       string `json:"state"`
	FollowUp    bool   `json:"follow_up"`
	DownloadURL string `json:"download_url,omitempty"`
}

func (h *matterHandoff) signedDelivery(ctx context.Context, in deliveryRequest) (deliveryDecision, error) {
	prefix := "matters/" + in.MatterID + "/signed/"
	if in.MatterID == "" || in.RequestID == "" || !strings.HasPrefix(in.ObjectKey, prefix) {
		return deliveryDecision{}, fmt.Errorf("matter_id, request_id, and a matching signed object_key are required")
	}
	head, err := h.storage.objectHead(ctx, h.bucket, in.ObjectKey)
	if err != nil {
		return deliveryDecision{}, err
	}
	if !head.Found {
		return deliveryDecision{State: "awaiting_signature", FollowUp: !in.Deadline.After(h.now().Add(24 * time.Hour))}, nil
	}
	signed, err := h.storage.presignGet(ctx, h.bucket, in.ObjectKey, "attachment", in.RequestID)
	if err != nil {
		return deliveryDecision{}, err
	}
	return deliveryDecision{State: "ready_for_delivery", DownloadURL: signed.URL}, nil
}
