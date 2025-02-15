package ginboot

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3FileService struct {
	s3Client      *s3.S3
	bucket        string
	expireTime    int
	localFilePath string
}

func NewS3FileService(bucket, localFilePath, accessKey, secretKey, region, defaultExpireTime string) *S3FileService {
	expireTime, err := strconv.Atoi(defaultExpireTime)
	if err != nil {
		log.Fatalf("Invalid expire time: %v", err)
	}

	// Initialize AWS session
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	}))

	s3Client := s3.New(sess)

	return &S3FileService{
		s3Client:      s3Client,
		bucket:        bucket,
		expireTime:    expireTime,
		localFilePath: localFilePath,
	}
}

func (s *S3FileService) IsExists(path string) bool {
	_, err := s.s3Client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == s3.ErrCodeNoSuchKey {
			return false
		}
		log.Fatalf("Failed to check file existence: %v", err)
	}
	return true
}

func (s *S3FileService) Download(path string) (io.ReadCloser, error) {
	result, err := s.s3Client.GetObject(&s3.GetObjectInput{
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

	_, err = s.s3Client.PutObject(&s3.PutObjectInput{
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
	_, err := s.s3Client.DeleteObject(&s3.DeleteObjectInput{
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
	req, _ := s.s3Client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	urlStr, err := req.Presign(time.Duration(expireTime) * time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}
	return urlStr, nil
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
	req, _ := s.s3Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(filePath),
	})

	// Expire in 10 minutes
	urlStr, err := req.Presign(10 * time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to generate upload presigned URL: %v", err)
	}
	return urlStr, nil
}
