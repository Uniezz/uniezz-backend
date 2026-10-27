package media

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage struct {
	client         *s3.Client
	presign        *s3.PresignClient
	bucket         string
	publicMediaURL string
}

type Kind string

const (
	KindAvatar Kind = "avatar"
	KindFeed   Kind = "feed"
	KindChat   Kind = "chat"

	uploadURLTTL          = 10 * time.Minute
	privateDownloadURLTTL = 10 * time.Minute
)

func maxUploadSize(kind Kind) (int64, error) {
	switch kind {
	case KindAvatar:
		return 5 * 1024 * 1024, nil
	case KindFeed, KindChat:
		return 10 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unsupported media kind %q", kind)
	}
}

func validateUpload(kind Kind, contentType string, size int64) error {
	maxSize, err := maxUploadSize(kind)
	if err != nil {
		return err
	}
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return fmt.Errorf("unsupported media content type %q", contentType)
	}
	if size <= 0 || size > maxSize {
		return fmt.Errorf("media size must be between 1 and %d bytes", maxSize)
	}
	return nil
}

func publicEndpoint(cfg *config.Config) string {
	if cfg.AWSPublicEndpoint != "" {
		return cfg.AWSPublicEndpoint
	}
	return cfg.AWSEndpoint
}

func (s *Storage) CheckBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("check S3 bucket %q: %w", s.bucket, err)
	}
	return nil
}

func (s *Storage) HeadObject(ctx context.Context, key string) (int64, string, error) {
	if key == "" {
		return 0, "", fmt.Errorf("media object key is required")
	}

	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, "", fmt.Errorf("check media object: %w", err)
	}

	return aws.ToInt64(output.ContentLength), aws.ToString(output.ContentType), nil
}

func newStorage(ctx context.Context, cfg *config.Config) (*Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("media config is required")
	}
	if cfg.AWSRegion == "" {
		return nil, fmt.Errorf("AWS_REGION is required")
	}
	if cfg.AWSS3Bucket == "" {
		return nil, fmt.Errorf("AWS_S3_BUCKET is required")
	}
	if cfg.AWSAccessKeyID == "" || cfg.AWSSecretAccessKey == "" {
		return nil, fmt.Errorf("AWS access key and secret key are required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AWSAccessKeyID,
				cfg.AWSSecretAccessKey,
				"",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.AWSEndpoint != "" {
			options.BaseEndpoint = aws.String(cfg.AWSEndpoint)
		}
		options.UsePathStyle = cfg.AWSS3ForcePathStyle
	})
	presignClient := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if endpoint := publicEndpoint(cfg); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
		options.UsePathStyle = cfg.AWSS3ForcePathStyle
	})

	storage := &Storage{
		client:         client,
		presign:        s3.NewPresignClient(presignClient),
		bucket:         cfg.AWSS3Bucket,
		publicMediaURL: cfg.AWSPublicMediaURL,
	}

	return storage, nil

}

func (s *Storage) PresignPutURL(
	ctx context.Context,
	key string,
	kind Kind,
	contentType string,
	size int64,
) (string, error) {
	if key == "" {
		return "", fmt.Errorf("media object key is required")
	}
	if err := validateUpload(kind, contentType, size); err != nil {
		return "", err
	}

	request, err := s.presign.PresignPutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket:        aws.String(s.bucket),
			Key:           aws.String(key),
			ContentType:   aws.String(contentType),
			ContentLength: aws.Int64(size),
		},
		func(options *s3.PresignOptions) {
			options.Expires = uploadURLTTL
		},
	)
	if err != nil {
		return "", fmt.Errorf("presign media upload: %w", err)
	}
	return request.URL, nil
}

func (s *Storage) PresignGetURL(
	ctx context.Context,
	key string,
) (string, error) {
	if key == "" {
		return "", fmt.Errorf("media object key is required")
	}

	request, err := s.presign.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		},
		func(options *s3.PresignOptions) {
			options.Expires = privateDownloadURLTTL
		},
	)
	if err != nil {
		return "", fmt.Errorf("presign media download: %w", err)
	}
	return request.URL, nil
}

func (s *Storage) PublicURL(key string) (string, error) {
	if s.publicMediaURL == "" {
		return "", fmt.Errorf("public media URL is not configured")
	}
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid media object key")
	}

	return strings.TrimRight(s.publicMediaURL, "/") + "/" + key, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("media object key is required")
	}

	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete media object: %w", err)
	}
	return nil
}
