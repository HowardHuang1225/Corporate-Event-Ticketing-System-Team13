package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)


type MinioService struct {
	client         *minio.Client
	bucketName     string
	publicEndpoint string
}

func NewMinioService(endpoint, accessKey, secretKey, bucketName, publicEndpoint string) *MinioService {
	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalf("Failed to initialize MinIO client: %v", err)
	}

	ctx := context.Background()
	
	// Create bucket if it doesn't exist
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Printf("Failed to check if bucket exists: %v", err)
	} else if !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Printf("Failed to create bucket: %v", err)
		} else {
			log.Printf("Successfully created bucket %s", bucketName)
			// Set public read policy
			policy := fmt.Sprintf(`{"Version": "2012-10-17","Statement": [{"Action": ["s3:GetObject"],"Effect": "Allow","Principal": {"AWS": ["*"]},"Resource": ["arn:aws:s3:::%s/*"]}]}`, bucketName)
			err = minioClient.SetBucketPolicy(ctx, bucketName, policy)
			if err != nil {
				log.Printf("Failed to set bucket policy: %v", err)
			}
		}
	}

	return &MinioService{
		client:         minioClient,
		bucketName:     bucketName,
		publicEndpoint: publicEndpoint,
	}
}

func (s *MinioService) UploadFile(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string, originalName string) (string, error) {
	// 限制上傳最大時間為 30 秒，防堵慢速攻擊 (Slowloris)
	uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	userMetadata := make(map[string]string)
	if originalName != "" {
		// 將過濾後的原始檔名存入 Metadata，兼顧安全性與使用者體驗
		userMetadata["original-name"] = originalName
	}

	_, err := s.client.PutObject(uploadCtx, s.bucketName, objectName, reader, objectSize, minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: userMetadata,
	})
	if err != nil {
		return "", err
	}
	
	// Return the public URL
	return fmt.Sprintf("%s/%s/%s", s.publicEndpoint, s.bucketName, objectName), nil
}
