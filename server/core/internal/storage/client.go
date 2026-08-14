// Package storage provides object storage access backed by MinIO.
package storage

import (
	"context"
	"fmt"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Options specifies the configuration for the client.
type Options struct {
	// Bucket specifies the bucket to use.
	Bucket string

	// URL specifies the url to the object service.
	URL string

	// PublicURL specifies the public-facing URL for accessing objects.
	// If empty, URL is used instead.
	PublicURL string

	// AccessKey specifies access key to the minio service.
	AccessKey string

	// SecretKey specifies secret key to the minio service.
	SecretKey string
}

// Client is a client to access buckets.
type Client struct {
	client *minio.Client
	bucket string
}

// NewClient creates a new client instance.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	mc, err := setupClient(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: mc,
		bucket: opts.Bucket,
	}, nil
}

// setupClient sets up a minio client with the given config.
func setupClient(ctx context.Context, opts Options) (*minio.Client, error) {
	u, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing node url: %w", err)
	}

	mc, err := minio.New(
		u.Host,
		&minio.Options{
			Secure: u.Scheme == "https",
			Creds: credentials.NewStaticV4(
				opts.AccessKey,
				opts.SecretKey,
				"",
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("creating minio client: %w", err)
	}

	ok, err := mc.BucketExists(ctx, opts.Bucket)
	if err != nil {
		return nil, fmt.Errorf("checking bucket: %w", err)
	}

	if !ok {
		err := mc.MakeBucket(ctx, opts.Bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("creating bucket: %w", err)
		}
	}

	return mc, nil
}
