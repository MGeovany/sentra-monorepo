package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint    string
	Region      string
	Bucket      string
	AccessKeyID string
	SecretKey   string
	SessionTok  string
	UseSSL      bool
}

func LoadS3ConfigFromEnv() (S3Config, bool, error) {
	if !envBool("SENTRA_BYOS") {
		return S3Config{}, false, nil
	}

	endpoint := strings.TrimSpace(os.Getenv("SENTRA_S3_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("SENTRA_S3_BUCKET"))
	region := strings.TrimSpace(os.Getenv("SENTRA_S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}

	ak := strings.TrimSpace(os.Getenv("SENTRA_S3_ACCESS_KEY_ID"))
	sk := strings.TrimSpace(os.Getenv("SENTRA_S3_SECRET_ACCESS_KEY"))
	st := strings.TrimSpace(os.Getenv("SENTRA_S3_SESSION_TOKEN"))
	if ak == "" {
		ak = strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	}
	if sk == "" {
		sk = strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	}
	if st == "" {
		st = strings.TrimSpace(os.Getenv("AWS_SESSION_TOKEN"))
	}

	useSSL := true
	if v := strings.TrimSpace(os.Getenv("SENTRA_S3_USE_SSL")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			useSSL = b
		}
	}

	if endpoint == "" || bucket == "" || ak == "" || sk == "" {
		return S3Config{}, false, errors.New("BYOS enabled but missing S3 env vars (need SENTRA_S3_ENDPOINT, SENTRA_S3_BUCKET, SENTRA_S3_ACCESS_KEY_ID, SENTRA_S3_SECRET_ACCESS_KEY)")
	}

	return S3Config{
		Endpoint:    endpoint,
		Region:      region,
		Bucket:      bucket,
		AccessKeyID: ak,
		SecretKey:   sk,
		SessionTok:  st,
		UseSSL:      useSSL,
	}, true, nil
}

func NewS3Client(cfg S3Config) (*minio.Client, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretKey, cfg.SessionTok),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	c, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func PutObject(ctx context.Context, client *minio.Client, cfg S3Config, key string, content []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r := bytes.NewReader(content)
	_, err := client.PutObject(ctx, cfg.Bucket, key, r, int64(len(content)), minio.PutObjectOptions{})
	return err
}

func GetObject(ctx context.Context, client *minio.Client, cfg S3Config, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	obj, err := client.GetObject(ctx, cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()

	b, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("empty object: %s", key)
	}
	return b, nil
}

func envBool(k string) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}
