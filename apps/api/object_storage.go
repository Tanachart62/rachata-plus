package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/gin-gonic/gin"
)

type objectStorage struct {
	client         *s3.Client
	signer         *s3.PresignClient
	bucket, prefix string
}

func newObjectStorage(ctx context.Context) (*objectStorage, error) {
	if envDefault("MEDIA_BACKEND", "local") == "local" {
		return nil, nil
	}
	if os.Getenv("MEDIA_BACKEND") != "s3" {
		return nil, fmt.Errorf("MEDIA_BACKEND must be local or s3")
	}
	bucket, prefix := os.Getenv("S3_BUCKET"), strings.Trim(os.Getenv("S3_PREFIX"), "/")
	if bucket == "" || prefix == "" || strings.Contains(prefix, "..") || strings.ContainsAny(prefix, "\\\r\n") {
		return nil, fmt.Errorf("S3_BUCKET and a nonempty S3_PREFIX are required")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		return nil, fmt.Errorf("AWS_REGION is required")
	}
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if os.Getenv("APP_ENV") == "production" && (endpoint != "" || os.Getenv("AWS_ENDPOINT_URL") != "" || os.Getenv("AWS_ENDPOINT_URL_S3") != "") {
		return nil, fmt.Errorf("custom S3 endpoints are forbidden in production")
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("initialize AWS SDK")
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})
	return &objectStorage{client: client, signer: s3.NewPresignClient(client), bucket: bucket, prefix: prefix}, nil
}
func (s *objectStorage) key(relative string) string {
	return s.prefix + "/" + strings.TrimLeft(filepath.ToSlash(relative), "/")
}
func (s *objectStorage) put(ctx context.Context, relative string, body io.ReadSeeker, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.key(relative)), Body: body, ContentLength: aws.Int64(size), ContentType: aws.String(contentType)})
	return err
}
func (s *objectStorage) bytes(ctx context.Context, relative string, data []byte, contentType string) error {
	return s.put(ctx, relative, bytes.NewReader(data), int64(len(data)), contentType)
}
func (s *objectStorage) read(ctx context.Context, relative string, limit int64) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.key(relative))})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	if aws.ToInt64(out.ContentLength) > limit {
		return nil, fmt.Errorf("object exceeds read budget")
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("object exceeds read budget")
	}
	return data, err
}
func (s *objectStorage) download(ctx context.Context, relative, dest string, limit int64) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.key(relative))})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	if aws.ToInt64(out.ContentLength) > limit {
		return errMediaBudget
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(file, io.LimitReader(out.Body, limit+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if n > limit {
		return errMediaBudget
	}
	return closeErr
}
func (s *objectStorage) uploadTree(ctx context.Context, root, relative string) error {
	return filepath.WalkDir(root, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in output")
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		contentType := "application/octet-stream"
		if strings.HasSuffix(rel, ".m3u8") {
			contentType = "application/vnd.apple.mpegurl"
		} else if strings.HasSuffix(rel, ".ts") {
			contentType = "video/mp2t"
		}
		return s.put(ctx, path.Join(relative, filepath.ToSlash(rel)), file, info.Size(), contentType)
	})
}
func (s *objectStorage) deletePrefix(ctx context.Context, relative string) error {
	prefix := s.key(relative) + "/"
	pager := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix)})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return err
		}
		objects := make([]types.ObjectIdentifier, 0, len(out.Contents))
		for _, item := range out.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: item.Key})
		}
		if len(objects) == 0 {
			continue
		}
		deleted, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: aws.String(s.bucket), Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)}})
		if err != nil {
			return err
		}
		if len(deleted.Errors) > 0 {
			return fmt.Errorf("object deletion incomplete")
		}
	}
	return nil
}
func objectError(c *gin.Context, err error) {
	var api smithy.APIError
	if errors.As(err, &api) && api.ErrorCode() == "InvalidRange" {
		c.Status(416)
		return
	}
	if errors.As(err, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound") {
		c.Status(404)
		return
	}
	c.Status(503)
}
func (s *objectStorage) serve(c *gin.Context, relative, contentType string) {
	input := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.key(relative))}
	if value := c.GetHeader("Range"); value != "" {
		// S3 supports one range per GET; pass only a bounded single bytes range.
		if len(value) > 100 || !regexp.MustCompile(`^bytes=(?:[0-9]+-[0-9]*|-[0-9]+)$`).MatchString(value) {
			c.Status(416)
			return
		}
		input.Range = aws.String(value)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	if c.Request.Method == http.MethodHead {
		out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: input.Bucket, Key: input.Key, Range: input.Range})
		if err != nil {
			objectError(c, err)
			return
		}
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", strconv.FormatInt(aws.ToInt64(out.ContentLength), 10))
		c.Header("Accept-Ranges", "bytes")
		c.Status(200)
		return
	}
	out, err := s.client.GetObject(ctx, input)
	if err != nil {
		objectError(c, err)
		return
	}
	defer out.Body.Close()
	status := 200
	if out.ContentRange != nil {
		status = 206
		c.Header("Content-Range", aws.ToString(out.ContentRange))
	}
	c.Header("Accept-Ranges", "bytes")
	c.DataFromReader(status, aws.ToInt64(out.ContentLength), contentType, out.Body, nil)
}
