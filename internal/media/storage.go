package media

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type objectStorage struct {
	client         *s3.Client
	presign        *s3.PresignClient
	bucket         string
	publicMediaURL string
}

func newObjectStorage(ctx context.Context, cfg *config.Config) (*objectStorage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKeyID, cfg.S3SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}

	return &objectStorage{
		client:         newS3Client(awsCfg, cfg.S3Endpoint, cfg.S3ForcePathStyle),
		presign:        s3.NewPresignClient(newS3Client(awsCfg, publicEndpoint(cfg), cfg.S3ForcePathStyle)),
		bucket:         cfg.S3Bucket,
		publicMediaURL: cfg.MediaPublicURL,
	}, nil
}

func newS3Client(awsCfg aws.Config, endpoint string, pathStyle bool) *s3.Client {
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = pathStyle
	})
}

func publicEndpoint(cfg *config.Config) string {
	if cfg.S3PublicEndpoint != "" {
		return cfg.S3PublicEndpoint
	}
	return cfg.S3Endpoint
}

func (s *objectStorage) checkBucket(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("check S3 bucket %q: %w", s.bucket, err)
	}
	return nil
}

func (s *objectStorage) headObject(ctx context.Context, key string) (int64, string, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return 0, "", ErrUploadMismatch
		}
		return 0, "", fmt.Errorf("head media object: %w", err)
	}
	return aws.ToInt64(out.ContentLength), aws.ToString(out.ContentType), nil
}

func (s *objectStorage) presignPut(ctx context.Context, key, contentType string, size int64) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}, s3.WithPresignExpires(uploadURLTTL))
	if err != nil {
		return "", fmt.Errorf("presign media upload: %w", err)
	}
	return req.URL, nil
}

func (s *objectStorage) presignGet(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(privateURLTTL))
	if err != nil {
		return "", fmt.Errorf("presign media download: %w", err)
	}
	return req.URL, nil
}

func (s *objectStorage) publicURL(key string) (string, error) {
	if s.publicMediaURL == "" {
		return "", errors.New("MEDIA_PUBLIC_URL is not configured")
	}
	return strings.TrimRight(s.publicMediaURL, "/") + "/" + key, nil
}
