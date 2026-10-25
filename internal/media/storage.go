package media

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
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

func New(ctx context.Context, cfg *config.Config) (*Storage, error) {
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
		client:  client,
		presign: s3.NewPresignClient(presignClient),
		bucket:  cfg.AWSS3Bucket,
	}

	if err := storage.CheckBucket(ctx); err != nil {
		return nil, err
	}

	return storage, nil

}

const maxPresignDuration = 7 * 24 * time.Hour

func validatePresignExpiry(expires time.Duration) error {
	if expires <= 0 || expires > maxPresignDuration {
		return fmt.Errorf("presigned URL expiry must be between 0 and 7 days")
	}
	return nil
}

func (s *Storage) PresignPutURL(
	ctx context.Context,
	key string,
	contentType string,
	expires time.Duration,
) (string, error) {
	if key == "" {
		return "", fmt.Errorf("media object key is required")
	}
	if contentType == "" {
		return "", fmt.Errorf("media content type is required")
	}
	if err := validatePresignExpiry(expires); err != nil {
		return "", err
	}

	request, err := s.presign.PresignPutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket:      aws.String(s.bucket),
			Key:         aws.String(key),
			ContentType: aws.String(contentType),
		},
		func(options *s3.PresignOptions) {
			options.Expires = expires
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
	expires time.Duration,
) (string, error) {
	if key == "" {
		return "", fmt.Errorf("media object key is required")
	}
	if err := validatePresignExpiry(expires); err != nil {
		return "", err
	}

	request, err := s.presign.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
		},
		func(options *s3.PresignOptions) {
			options.Expires = expires
		},
	)
	if err != nil {
		return "", fmt.Errorf("presign media download: %w", err)
	}
	return request.URL, nil
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
