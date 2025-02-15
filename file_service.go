package ginboot

import "io"

type FileService interface {
	IsExists(path string) bool
	Download(path string) (io.ReadCloser, error)
	Upload(localPath, remotePath string) error
	Delete(path string) error
	GetURL(path string) (string, error)
	GetURLWithExpiry(path string, expireTime int) (string, error)
	DeleteLocalFile(path string) error
	GetUploadURL(fileName, path string) (string, error)
}
