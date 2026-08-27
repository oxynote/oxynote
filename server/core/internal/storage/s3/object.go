package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/oxynote/oxynote/server/core/internal/storage"
)

// Upload uploads a new object.
// If the object with the same ID already exists, it is overwritten.
func (c *Client) Upload(ctx context.Context, folder, id string, r io.Reader) error {
	data, ct, err := storage.ReadObject(r)
	if err != nil {
		return err
	}

	_, err = c.client.PutObject(
		ctx,
		&awss3.PutObjectInput{
			Bucket:        aws.String(c.bucket),
			Key:           aws.String(path.Join(folder, id)),
			Body:          bytes.NewReader(data),
			ContentLength: aws.Int64(int64(len(data))),
			ContentType:   aws.String(ct),
		},
	)
	if err != nil {
		return fmt.Errorf("putting object: %w", err)
	}

	return nil
}

// Retrieve retrieves an object by its ID.
func (c *Client) Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error) {
	obj, err := c.client.GetObject(
		ctx,
		&awss3.GetObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(path.Join(folder, id)),
		},
	)

	var nsk *types.NoSuchKey

	switch {
	case err == nil:
		// OK.
	case errors.As(err, &nsk):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("getting object: %w", err)
	}

	return &storage.ObjectInfo{
		Body: obj.Body,
		// S3 quotes the entity tag; the value is served back as a bare
		// ETag header and compared against If-None-Match as such.
		ETag:        strings.Trim(aws.ToString(obj.ETag), `"`),
		ContentType: aws.ToString(obj.ContentType),
	}, true, nil
}

// Copy copies an object within the bucket, server-side. The object is never
// streamed through this process: uploads are capped well below the multipart
// threshold, so a copy stays a metadata-only operation.
func (c *Client) Copy(ctx context.Context, srcFolder, srcID, dstFolder, dstID string) error {
	_, err := c.client.CopyObject(
		ctx,
		&awss3.CopyObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(path.Join(dstFolder, dstID)),
			// the source is named as a single "<bucket>/<key>" path and
			// carries the key's own escaping.
			CopySource: aws.String(url.PathEscape(path.Join(c.bucket, srcFolder, srcID))),
		},
	)
	if err != nil {
		return fmt.Errorf("copying object: %w", err)
	}

	return nil
}

// Delete deletes an object by its ID. Deleting an object that is not there
// is not an error, which is what lets a crashed upload heal: the row that
// outlived it is removed on the next cleanup pass either way.
func (c *Client) Delete(ctx context.Context, folder, id string) error {
	_, err := c.client.DeleteObject(
		ctx,
		&awss3.DeleteObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(path.Join(folder, id)),
		},
	)
	if err != nil {
		return fmt.Errorf("removing object: %w", err)
	}

	return nil
}
