package ossstorage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/luuuunet/owpanel/internal/models"
)

type s3Store struct {
	client *s3.Client
	bucket string
	prefix string
	name   string
}

func newS3Store(st *models.OSSStorage) (*s3Store, error) {
	if strings.TrimSpace(st.Bucket) == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	endpoint := ResolveEndpoint(st.Provider, st.Region, st.Endpoint)
	region := strings.TrimSpace(st.Region)
	if region == "" || region == "auto" {
		region = "us-east-1"
	}
	pathStyle := st.UsePathStyle || DefaultPathStyle(st.Provider)
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(st.AccessKey, st.SecretKey, ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(strings.TrimRight(endpoint, "/"))
		}
		o.UsePathStyle = pathStyle
	})
	return &s3Store{
		client: client,
		bucket: st.Bucket,
		prefix: strings.Trim(st.PathPrefix, "/"),
		name:   st.Name,
	}, nil
}

func (s *s3Store) DisplayName() string { return s.name }

func (s *s3Store) fullKey(key string) string {
	return joinKey(s.prefix, key)
}

func (s *s3Store) Test() error {
	ctx := context.Background()
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

func (s *s3Store) List(prefix string, limit int) ([]ObjectInfo, error) {
	ctx := context.Background()
	p := s.fullKey(prefix)
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(p),
		MaxKeys: aws.Int32(int32(max(1, limit))),
	})
	if err != nil {
		return nil, err
	}
	items := make([]ObjectInfo, 0, len(out.Contents))
	for _, obj := range out.Contents {
		key := aws.ToString(obj.Key)
		if s.prefix != "" {
			key = strings.TrimPrefix(key, s.prefix+"/")
		}
		items = append(items, ObjectInfo{
			Key:          key,
			Size:         aws.ToInt64(obj.Size),
			LastModified: obj.LastModified.Format(time.RFC3339),
		})
	}
	return items, nil
}

func (s *s3Store) UploadFile(localPath, key string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	ctx := context.Background()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
		Body:   f,
	})
	return err
}

func (s *s3Store) DownloadFile(key, localPath string) error {
	ctx := context.Background()
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, out.Body)
	return err
}

func (s *s3Store) Delete(key string) error {
	ctx := context.Background()
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	})
	return err
}

func (s *s3Store) Walk(prefix string, fn func(ObjectInfo) error) error {
	ctx := context.Background()
	p := s.fullKey(prefix)
	pager := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(p),
	})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			key := aws.ToString(obj.Key)
			if strings.HasSuffix(key, "/") {
				continue
			}
			if s.prefix != "" {
				key = strings.TrimPrefix(key, s.prefix+"/")
			}
			if err := fn(ObjectInfo{Key: key, Size: aws.ToInt64(obj.Size)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *s3Store) copyObject(srcKey, dstKey string) error {
	ctx := context.Background()
	src := s.fullKey(srcKey)
	dst := s.fullKey(dstKey)
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(fmt.Sprintf("%s/%s", s.bucket, src)),
		Key:        aws.String(dst),
	})
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
