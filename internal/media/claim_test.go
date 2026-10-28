package media

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakePendingStore struct {
	mediaStore
	item        *Media
	getErr      error
	markedReady bool
}

func (f *fakePendingStore) getOwnedPending(context.Context, uuid.UUID, uuid.UUID, Kind) (*Media, error) {
	return f.item, f.getErr
}

func (f *fakePendingStore) markReady(context.Context, uuid.UUID, uuid.UUID) error {
	f.markedReady = true
	return nil
}

type fakeInspector struct {
	objectStore
	size        int64
	contentType string
	err         error
	called      bool
}

func (f *fakeInspector) headObject(context.Context, string) (int64, string, error) {
	f.called = true
	return f.size, f.contentType, f.err
}

func TestClaim(t *testing.T) {
	storageErr := errors.New("storage unavailable")

	tests := []struct {
		name       string
		getErr     error
		headErr    error
		size       int64
		ctype      string
		wantErr    error
		wantHead   bool
		wantMarked bool
	}{
		{name: "claims matching object", size: 1024, ctype: "image/jpeg", wantHead: true, wantMarked: true},
		{name: "rejects other size", size: 2048, ctype: "image/jpeg", wantErr: ErrUploadMismatch, wantHead: true},
		{name: "rejects other content type", size: 1024, ctype: "image/png", wantErr: ErrUploadMismatch, wantHead: true},
		{name: "returns storage error", headErr: storageErr, wantErr: storageErr, wantHead: true},
		{name: "returns not found before touching storage", getErr: ErrNotFound, wantErr: ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakePendingStore{
				item:   &Media{ObjectKey: "avatars/test.jpg", ContentType: "image/jpeg", SizeBytes: 1024},
				getErr: tt.getErr,
			}
			inspector := &fakeInspector{size: tt.size, contentType: tt.ctype, err: tt.headErr}

			err := Module{repo: store, storage: inspector}.Claim(context.Background(), uuid.New(), uuid.New(), KindAvatar)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Claim() error = %v, want %v", err, tt.wantErr)
			}
			if inspector.called != tt.wantHead {
				t.Errorf("headObject called = %v, want %v", inspector.called, tt.wantHead)
			}
			if store.markedReady != tt.wantMarked {
				t.Errorf("markedReady = %v, want %v", store.markedReady, tt.wantMarked)
			}
		})
	}
}
