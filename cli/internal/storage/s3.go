package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint string
	Region   string
	Bucket   string
	UseSSL   bool

	Creds *credentials.Credentials
}

func NewS3Client(cfg S3Config) (*minio.Client, error) {
	if cfg.Creds == nil {
		cfg.Creds = credentials.NewEnvAWS()
	}
	opts := &minio.Options{
		Creds:  cfg.Creds,
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	c, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ResolveS3 returns a configured S3 client.
//
// Rule: if ~/.sentra/storage.json exists, BYOS is active.
// Dev fallback: if SENTRA_S3_BUCKET and SENTRA_S3_ENDPOINT are set, BYOS is active.
func ResolveS3() (cfg S3Config, client *minio.Client, enabled bool, err error) {
	if c, ok, err := LoadConfig(); err != nil {
		return S3Config{}, nil, false, err
	} else if ok {
		cfg, err = s3ConfigFromStored(c)
		if err != nil {
			return S3Config{}, nil, false, err
		}
		client, err = NewS3Client(cfg)
		if err != nil {
			return S3Config{}, nil, false, err
		}
		return cfg, client, true, nil
	}

	// Dev env fallback
	endpoint := strings.TrimSpace(os.Getenv("SENTRA_S3_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("SENTRA_S3_BUCKET"))
	if endpoint == "" || bucket == "" {
		return S3Config{}, nil, false, nil
	}
	region := strings.TrimSpace(os.Getenv("SENTRA_S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}

	cfg = S3Config{
		Endpoint: endpoint,
		Region:   region,
		Bucket:   bucket,
		UseSSL:   strings.TrimSpace(os.Getenv("SENTRA_S3_USE_SSL")) != "false",
		Creds:    credentials.NewEnvAWS(),
	}
	client, err = NewS3Client(cfg)
	if err != nil {
		return S3Config{}, nil, false, err
	}
	return cfg, client, true, nil
}

// ResolveS3FromConfig is used by `sentra storage setup` to test before saving.
func ResolveS3FromConfig(c Config) (cfg S3Config, client *minio.Client, enabled bool, err error) {
	cfg, err = s3ConfigFromStored(c)
	if err != nil {
		return S3Config{}, nil, false, err
	}
	client, err = NewS3Client(cfg)
	if err != nil {
		return S3Config{}, nil, false, err
	}
	return cfg, client, true, nil
}

func s3ConfigFromStored(c Config) (S3Config, error) {
	if err := c.Validate(); err != nil {
		return S3Config{}, err
	}

	endpoint := strings.TrimSpace(c.Endpoint)
	region := strings.TrimSpace(c.Region)
	// AWS: avoid the global endpoint when bucket is regional.
	if c.Provider == ProviderAWSS3 && endpoint == "s3.amazonaws.com" && region != "" && region != "us-east-1" {
		endpoint = "s3." + region + ".amazonaws.com"
	}

	var creds *credentials.Credentials
	switch c.AuthMethod {
	case AuthAWSProfile:
		creds = credentials.NewFileAWSCredentials(strings.TrimSpace(c.AWSCredentialsFile), strings.TrimSpace(c.AWSProfile))
	case AuthEnvOnly:
		creds = credentials.NewEnvAWS()
	case AuthStatic:
		secretKey := ""
		sessionToken := ""
		switch c.SecretLocation {
		case SecretKeyring:
			sk, st, ok, err := loadSecret(strings.TrimSpace(c.SecretRef))
			if err != nil {
				return S3Config{}, err
			}
			if !ok {
				return S3Config{}, fmt.Errorf("missing credentials in keychain for %s", c.SecretRef)
			}
			secretKey = sk
			sessionToken = st
		case SecretFile:
			secretKey = strings.TrimSpace(c.SecretKey)
			sessionToken = strings.TrimSpace(c.SessionToken)
		default:
			return S3Config{}, fmt.Errorf("invalid secret_location")
		}
		creds = credentials.NewStaticV4(strings.TrimSpace(c.AccessKeyID), secretKey, sessionToken)
	default:
		return S3Config{}, fmt.Errorf("invalid auth_method")
	}

	return S3Config{
		Endpoint: endpoint,
		Region:   region,
		Bucket:   strings.TrimSpace(c.Bucket),
		UseSSL:   c.UseSSL,
		Creds:    creds,
	}, nil
}

func TestS3(ctx context.Context, client *minio.Client, cfg S3Config) error {
	key := "sentra/test/" + uuid.NewString() + ".bin"
	payload := []byte("sentra-test")

	if err := PutObject(ctx, client, cfg, key, payload); err != nil {
		return err
	}

	out, err := GetObject(ctx, client, cfg, key)
	if err != nil {
		return err
	}
	if string(out) != string(payload) {
		return fmt.Errorf("download mismatch")
	}

	// Best-effort cleanup.
	_ = client.RemoveObject(ctx, cfg.Bucket, key, minio.RemoveObjectOptions{})
	return nil
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
