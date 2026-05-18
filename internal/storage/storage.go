package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"recallix/internal/config"
)

// FileStorage 是 Recallix 的原始文件存储实现。
// 当前项目统一使用 MinIO，不再支持本地文件系统落盘。
type FileStorage struct {
	client     *minio.Client
	bucketName string
}

func New(cfg *config.Config) (*FileStorage, error) {
	if cfg.MinIOEndpoint == "" || cfg.MinIOAccessKeyID == "" ||
		cfg.MinIOSecretAccessKey == "" || cfg.MinIOBucketName == "" {
		return nil, fmt.Errorf("missing MinIO configuration")
	}

	client, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKeyID, cfg.MinIOSecretAccessKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize MinIO client: %w", err)
	}

	store := &FileStorage{client: client, bucketName: cfg.MinIOBucketName}
	exists, err := client.BucketExists(context.Background(), cfg.MinIOBucketName)
	if err != nil {
		return nil, fmt.Errorf("check MinIO bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("MinIO bucket %q does not exist", cfg.MinIOBucketName)
	}
	return store, nil
}

// Save 将原始文件写入 MinIO，并返回可持久化的对象 URI。
func (s *FileStorage) Save(ctx context.Context, objectKey, contentType string, size int64, reader io.Reader) (string, error) {
	if err := validateObjectKey(objectKey); err != nil {
		return "", err
	}
	if _, err := s.client.PutObject(ctx, s.bucketName, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return "", fmt.Errorf("upload file to MinIO: %w", err)
	}
	return fmt.Sprintf("minio://%s/%s", s.bucketName, objectKey), nil
}

func (s *FileStorage) Open(ctx context.Context, filePath string) (io.ReadCloser, error) {
	objectKey, err := s.parseURI(filePath)
	if err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get file from MinIO: %w", err)
	}
	return obj, nil
}

func (s *FileStorage) Delete(ctx context.Context, filePath string) error {
	objectKey, err := s.parseURI(filePath)
	if err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucketName, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete file from MinIO: %w", err)
	}
	return nil
}

func (s *FileStorage) parseURI(filePath string) (string, error) {
	const prefix = "minio://"
	if !strings.HasPrefix(filePath, prefix) {
		return "", fmt.Errorf("invalid MinIO file URI: %s", filePath)
	}
	rest := strings.TrimPrefix(filePath, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid MinIO file URI: %s", filePath)
	}
	if parts[0] != s.bucketName {
		return "", fmt.Errorf("MinIO bucket mismatch: got %q want %q", parts[0], s.bucketName)
	}
	if err := validateObjectKey(parts[1]); err != nil {
		return "", err
	}
	return parts[1], nil
}

func validateObjectKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, `\`) {
		return fmt.Errorf("invalid MinIO object key: %q", key)
	}
	cleaned := path.Clean(key)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("invalid MinIO object key: %q", key)
	}
	return nil
}
