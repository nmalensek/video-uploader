package vimeo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nmalensek/video-uploader/internal/app/database"
	"github.com/nmalensek/video-uploader/internal/app/database/filedb"
)

// Settings contains the PAT and settings used for video uploads.
type Settings struct {
	PersonalAccessToken string         `yaml:"personal_access_token"`
	UploadSettings      UploadSettings `yaml:"upload_settings"`
}

// UploadSettings are video-specific settings that must be set for new uploads.
type UploadSettings struct {
	ContentRating []string `yaml:"content_rating"`
	Privacy       Privacy  `yaml:"privacy"`
}

// Privacy defines who can access the uploaded video.
type Privacy struct {
	Comments string `yaml:"comments" json:"comments"`
	Embed    string `yaml:"embed" json:"embed"`
	View     string `yaml:"view" json:"view"`
	Download bool   `json:"download"`
}

// UploadData holds everything needed for an upload.
type UploadData struct {
	VideoName        string // May be redundant if using the filename as video name
	VideoDescription string
	Filename         string
	FilePath         string
	Password         string
	FileSize         int64
	ChunkSize        int
}

// UploadApproachSize contains the fields needed to start a tus upload.
type UploadApproachSize struct {
	Approach string `json:"approach"`
	Size     string `json:"size"`
}

// UploadPayload is the JSON payload.
type UploadPayload struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Password    string  `json:"password"`
	Privacy     Privacy `json:"privacy"`
	// the folder to upload the video to
	FolderURI     string             `json:"folder_uri"`
	ContentRating []string           `json:"content_rating"`
	Upload        UploadApproachSize `json:"upload"`
}

// UploadLink contains the fields returned from the POST initiating a tus upload.
type UploadLink struct {
	UploadLink string `json:"upload_link"`
}

// TUSResponse is a container for the fields in a tus initiation response.
type TUSResponse struct {
	FinalURI string `json:"uri"`
	TusURI   string `json:"upload"`
}

// Uploader uploads videos.
type Uploader struct {
	client   httpCaller
	settings Settings
	uploadDB database.UploadDatastore
}

type httpCaller interface {
	Do(*http.Request) (*http.Response, error)
}

const (
	uploadURI = "https://api.vimeo.com/me/videos"
)

func NewUploader(outputFolderPath string, hc httpCaller, s Settings) (Uploader, error) {
	uploadDBConn, err := filedb.New(outputFolderPath)
	if err != nil {
		return Uploader{}, err
	}

	return Uploader{
		client:   hc,
		settings: s,
		uploadDB: uploadDBConn,
	}, nil
}

func (u Uploader) Upload(data UploadData) error {
	// check for existing file in tracking file (failed initial upload case)
	r, err := u.uploadDB.GetUpload(data.Filename)
	if err != nil {
		fmt.Printf("WARN: error checking for prior upload, attempting upload. error: %v\n", err)
	}

	// uploadOffset := 0

	// if it's a new upload, make a call to set up all the base information
	if r.IsEmpty() {
		initialResp, err := initiateUpload(u.client, data, u.settings.PersonalAccessToken)
		if err != nil {
			// logging handled in called function.
			return err
		}

		r.Name = data.VideoName
		r.Status = database.InProgress
		r.TusURI = initialResp.TusURI
		r.VideoURI = uploadURI + initialResp.FinalURI

		saveErr := u.uploadDB.PutUpload(r)
		if saveErr != nil {
			return fmt.Errorf("started upload but error saving initial data: %v\ndata from vimeo:\n%v\n%v\n%v\n%v",
				saveErr, r.Name, r.Status, r.TusURI, r.VideoURI)
		}
	}

	// if it's not new, continue uploading from specified byte position

	return nil
}

func initiateUpload(c httpCaller, d UploadData, pat string) (TUSResponse, error) {
	payload := UploadPayload{
		Name:        d.VideoName,
		Description: d.VideoDescription,
		Password:    d.Password,
		Privacy: Privacy{
			Comments: "nobody",
			Embed:    "private",
			View:     "password",
			Download: false,
		},
		FolderURI:     "", // TODO
		ContentRating: []string{"unrated"},
		Upload: UploadApproachSize{
			Approach: "tus",
			Size:     fmt.Sprint(d.FileSize),
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return TUSResponse{}, fmt.Errorf("unable to prepare video payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, uploadURI, bytes.NewReader(bodyBytes))
	if err != nil {
		return TUSResponse{}, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/vnd.vimeo.*+json;version=3.4")
	req.Header.Add("Authorization", fmt.Sprintf("bearer %v", pat))

	retries := 0

	for retries < 2 {
		resp, err := c.Do(req)
		if err != nil {
			return TUSResponse{}, fmt.Errorf("error making post to vimeo upload URI: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(time.Second * 60) // TODO: calculate time remaining
			retries++
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return TUSResponse{}, fmt.Errorf("could not read initiation response bytes: %v", err)
		}

		var tResp TUSResponse
		err = json.Unmarshal(respBytes, &tResp)
		if err != nil {
			return TUSResponse{}, fmt.Errorf("could not unmarshal initiation response: %v", err)
		}
	}

	return TUSResponse{}, errors.New("failed to upload three times, aborting...")
}

// description
// name
// password
// upload.approach
// upload.size
