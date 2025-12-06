package ginboot

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3FileService struct {
	s3Client      *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	expireTime    int
	localFilePath string
}

func NewS3FileServiceWithConfig(cfg aws.Config, bucket, localFilePath, defaultExpireTime string) *S3FileService {
	expireTime, err := strconv.Atoi(defaultExpireTime)
	if err != nil {
		log.Fatalf("Invalid expire time: %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	presignClient := s3.NewPresignClient(s3Client)

	return &S3FileService{
		s3Client:      s3Client,
		presignClient: presignClient,
		bucket:        bucket,
		expireTime:    expireTime,
		localFilePath: localFilePath,
	}
}

func NewS3FileService(ctx context.Context, bucket, localFilePath, accessKey, secretKey, region, defaultExpireTime string) *S3FileService {
	// Initialize AWS session with static credentials for backward compatibility
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	return NewS3FileServiceWithConfig(cfg, bucket, localFilePath, defaultExpireTime)
}

func (s *S3FileService) IsExists(path string) bool {
	_, err := s.s3Client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})

	if err != nil {
		// In V2, check for NotFound error differently if needed, but usually error is enough
		return false
	}
	return true
}

func (s *S3FileService) Download(path string) (io.ReadCloser, error) {
	result, err := s.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}
	return result.Body, nil
}

func (s *S3FileService) Upload(localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", localPath, err)
	}
	defer file.Close()

	_, err = s.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(remotePath),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file: %v", err)
	}

	log.Printf("File %s uploaded to bucket %s successfully", remotePath, s.bucket)
	s.DeleteLocalFile(localPath)
	return nil
}

func (s *S3FileService) Delete(path string) error {
	_, err := s.s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("failed to delete file %s: %v", path, err)
	}
	return nil
}

func (s *S3FileService) GetURL(path string) (string, error) {
	return s.GetURLWithExpiry(path, s.expireTime)
}

func (s *S3FileService) GetURLWithExpiry(path string, expireTime int) (string, error) {
	req, err := s.presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	}, func(o *s3.PresignOptions) {
		o.Expires = time.Duration(expireTime) * time.Second
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}
	return req.URL, nil
}

func (s *S3FileService) DeleteLocalFile(path string) error {
	err := os.Remove(path)
	if err != nil {
		return fmt.Errorf("failed to delete local file %s: %v", path, err)
	}
	return nil
}

func (s *S3FileService) GetUploadURL(fileName, path string) (string, error) {
	filePath := filepath.Join(path, fileName)
	req, err := s.presignClient.PresignPutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(filePath),
	}, func(o *s3.PresignOptions) {
		o.Expires = 10 * time.Minute
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate upload presigned URL: %v", err)
	}
	return req.URL, nil
}
