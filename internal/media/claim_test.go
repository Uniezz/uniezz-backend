package media

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type claimTestRepository struct {
	item        *Media
	getErr      error
	markErr     error
	markedReady bool
}

func (r *claimTestRepository) getOwnedPending(
	_ context.Context,
	_, _ uuid.UUID,
	_ Kind,
) (*Media, error) {
	return r.item, r.getErr
}

func (r *claimTestRepository) markReady(
	_ context.Context,
	_, _ uuid.UUID,
) error {
	if r.markErr != nil {
		return r.markErr
	}
	r.markedReady = true
	return nil
}

type claimTestStorage struct {
	size        int64
	contentType string
	err         error
	called      bool
}

func (s *claimTestStorage) HeadObject(
	_ context.Context,
	_ string,
) (int64, string, error) {
	s.called = true
	return s.size, s.contentType, s.err
}

func TestClaimUploadedMedia(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	repositoryErr := errors.New("database unavailable")

	tests := []struct {
		name          string
		repositoryErr error
		storageErr    error
		actualSize    int64
		actualType    string
		wantError     bool
		wantHeadCall  bool
		wantReady     bool
	}{
		{
			name:         "claims matching uploaded object",
			actualSize:   1024,
			actualType:   "image/jpeg",
			wantHeadCall: true,
			wantReady:    true,
		},
		{
			name:         "rejects mismatched size",
			actualSize:   2048,
			actualType:   "image/jpeg",
			wantError:    true,
			wantHeadCall: true,
		},
		{
			name:         "rejects mismatched content type",
			actualSize:   1024,
			actualType:   "image/png",
			wantError:    true,
			wantHeadCall: true,
		},
		{
			name:         "returns storage error",
			storageErr:   storageErr,
			wantError:    true,
			wantHeadCall: true,
		},
		{
			name:          "returns repository error",
			repositoryErr: repositoryErr,
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mediaID := uuid.New()
			ownerID := uuid.New()

			repo := &claimTestRepository{
				item: &Media{
					ID:          mediaID,
					OwnerUserID: ownerID,
					Kind:        KindAvatar,
					ObjectKey:   "avatars/test.jpg",
					ContentType: "image/jpeg",
					SizeBytes:   1024,
					Status:      "pending",
				},
				getErr: tt.repositoryErr,
			}
			storage := &claimTestStorage{
				size:        tt.actualSize,
				contentType: tt.actualType,
				err:         tt.storageErr,
			}

			err := claimUploadedMedia(
				context.Background(),
				repo,
				storage,
				mediaID,
				ownerID,
				KindAvatar,
			)

			if (err != nil) != tt.wantError {
				t.Fatalf("claimUploadedMedia() error = %v, wantError %v", err, tt.wantError)
			}
			if storage.called != tt.wantHeadCall {
				t.Errorf("HeadObject called = %v, want %v", storage.called, tt.wantHeadCall)
			}
			if repo.markedReady != tt.wantReady {
				t.Errorf("markedReady = %v, want %v", repo.markedReady, tt.wantReady)
			}
		})
	}
}
