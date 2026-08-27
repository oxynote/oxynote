// Package s3 provides object storage access backed by an S3-compatible
// object store.
package s3

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// _defaultRegion is the region handed to the request signer when none is
// configured. SigV4 refuses to sign without one, and S3-compatible object
// stores that have no concept of regions accept anything.
const _defaultRegion = "us-east-1"

// Options specifies the configuration for the client.
type Options struct {
	// Bucket specifies the bucket to use.
	Bucket string

	// URL specifies the url to the object service.
	URL string

	// Region specifies the region the bucket lives in. Zero value defaults
	// to us-east-1, which suits stores that do not implement regions.
	Region string

	// AccessKey specifies access key to the object service.
	AccessKey string

	// SecretKey specifies secret key to the object service.
	SecretKey string
}

// Client is a client to access buckets.
type Client struct {
	client *awss3.Client
	bucket string
}

// NewClient creates a new client instance.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	sc, err := setupClient(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: sc,
		bucket: opts.Bucket,
	}, nil
}

// setupClient sets up an S3 client with the given config.
func setupClient(ctx context.Context, opts Options) (*awss3.Client, error) {
	u, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing node url: %w", err)
	}

	// without a host the SDK resolves its own AWS endpoints, which would
	// point a misconfigured deployment at the real S3 instead of failing.
	if u.Host == "" {
		return nil, errors.New("object service url has no host")
	}

	region := opts.Region
	if region == "" {
		region = _defaultRegion
	}

	sc := awss3.New(awss3.Options{
		BaseEndpoint: aws.String(opts.URL),
		Region:       region,
		UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(
			opts.AccessKey,
			opts.SecretKey,
			"",
		),
		// the default is to attach a CRC32 checksum to every request and
		// demand one on every response, which S3-compatible stores are not
		// obliged to implement.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})

	_, err = sc.HeadBucket(
		ctx,
		&awss3.HeadBucketInput{
			Bucket: aws.String(opts.Bucket),
		},
	)

	var nf *types.NotFound

	switch {
	case err == nil:
		// OK.
	case errors.As(err, &nf):
		_, err = sc.CreateBucket(
			ctx,
			&awss3.CreateBucketInput{
				Bucket: aws.String(opts.Bucket),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("creating bucket: %w", err)
		}
	default:
		return nil, fmt.Errorf("checking bucket: %w", err)
	}

	return sc, nil
}
