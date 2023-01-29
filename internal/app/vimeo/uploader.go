package vimeo

import "net/http"

// Settings contains the PAT and settings used for video uploads.
type Settings struct {
	PersonalAccessToken string         `yaml:"personal_access_token"`
	UploadSettings      UploadSettings `yaml:"upload_settings"`
}

// UploadSettings are video-specific settings that must be set for new uploads.
type UploadSettings struct {
	ContentRating string  `yaml:"content_rating"`
	Privacy       Privacy `yaml:"privacy"`
}

// Privacy defines who can access the uploaded video.
type Privacy struct {
	Comments string `yaml:"comments"`
	Embed    string `yaml:"embed"`
	View     string `yaml:"view"`
}

type Uploader struct {
	client   http.Client
	settings Settings
}

func NewUploader() Uploader {
	return Uploader{}
}

func (u Uploader) Upload(fileName, filePath, password string, fileSize int64, chunkSize int) error {
	return nil
}
