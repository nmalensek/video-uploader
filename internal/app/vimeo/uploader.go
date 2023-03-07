package vimeo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	Add      string `yaml:"add" json:"add"`
	Comments string `yaml:"comments" json:"comments"`
	Embed    string `yaml:"embed" json:"embed"`
	View     string `yaml:"view" json:"view"`
	Download *bool  `json:"download,omitempty"`
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
	Password    string  `json:"password,omitempty"`
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
	Name        string    `json:"name"`
	Description string    `json:"description"`
	FinalURI    string    `json:"uri"`
	Upload      TUSUpload `json:"upload"`
}

// TUSUpload contains the fields returned from a successful tus initation call's "upload" node.
type TUSUpload struct {
	Status     string `json:"status"`
	UploadLink string `json:"upload_link"`
	Approach   string `json:"approach"`
	Size       int    `json:"size"`
}

// Uploader uploads videos.
type Uploader struct {
	// client is used for all HTTP calls except uploading video chunks.
	client httpCaller
	// uploadClient is used to upload video chunks so it has a long timeout property.
	uploadClient httpCaller
	settings     Settings
	uploadDB     database.UploadDatastore
}

type httpCaller interface {
	Do(*http.Request) (*http.Response, error)
}

const (
	uploadURI     = "https://api.vimeo.com/me/videos"
	uploadFilters = "?fields=name,description,upload,uri"
	UploadOffset  = "Upload-Offset"
)

func NewUploader(outputFolderPath string, hc httpCaller, uhc httpCaller, s Settings) (Uploader, error) {
	uploadDBConn, err := filedb.New(outputFolderPath)
	if err != nil {
		return Uploader{}, err
	}

	return Uploader{
		client:       hc,
		uploadClient: uhc,
		settings:     s,
		uploadDB:     uploadDBConn,
	}, nil
}

func (u Uploader) Upload(data UploadData) error {
	// check for existing file in tracking file (failed initial upload case)
	r, err := u.uploadDB.GetUpload(strings.TrimSuffix(strings.TrimSuffix(data.Filename, ".mp4"), ".mov"))
	if err != nil {
		fmt.Printf("WARN: error checking for prior upload, attempting upload. error: %v\n", err)
	}

	var uploadOffset int64

	// if it's a new upload, make a call to set up all the base information
	if r.IsEmpty() {
		initialResp, err := initiateUpload(u.client, data, u.settings)
		if err != nil {
			// logging handled in called function.
			return err
		}

		// currently, using the filename as the video name, but saving what was calculated for metrics.
		r.Name = strings.TrimSuffix(data.Filename, ".mp4")
		r.CalculatedName = data.VideoName
		r.Status = database.InProgress
		r.TusURI = initialResp.Upload.UploadLink
		r.VideoURI = "https://vimeo.com" + initialResp.FinalURI

		saveErr := u.uploadDB.PutUpload(r)
		if saveErr != nil {
			return fmt.Errorf("started upload but error saving initial data: %v\ndata from vimeo:\n%v\n%v\n%v\n%v",
				saveErr, r.Name, r.Status, r.TusURI, r.VideoURI)
		}
	} else {
		if r.Status == database.Complete {
			fmt.Printf("file %v was already uploaded, skipping...\n", data.Filename)
			return nil
		}

		tempOffset, oErr := getOffset(u.client, r.TusURI)
		if oErr != nil {
			return fmt.Errorf("could not get offset for video %v: %v", r.Name, err)
		}

		if tempOffset == data.FileSize {
			r.Status = database.Complete
			pErr := u.uploadDB.PutUpload(r)
			if pErr != nil {
				fmt.Printf("error updating file %v status locally but the upload succeeded: %v", data.Filename, err)
			}
			fmt.Printf("file %v was already uploaded, skipping...\n", data.Filename)
			return nil
		}

		uploadOffset = tempOffset
	}

	err = uploadFromOffset(u.uploadClient, uploadOffset, r.TusURI, data.FilePath, data.ChunkSize, data.FileSize)
	if err != nil {
		return fmt.Errorf("error uploading file %v: %v", data.Filename, err)
	}

	fmt.Println("------------------------------")
	fmt.Printf("finished uploading file: \n%v\nvideo link: %v\npassword: %v\n", data.Filename, r.VideoURI, data.Password)
	fmt.Println("------------------------------")

	r.Status = database.Complete
	pErr := u.uploadDB.PutUpload(r)
	if pErr != nil {
		fmt.Printf("error updating file %v status locally but the upload succeeded: %v", data.Filename, err)
	}

	return nil
}

func initiateUpload(c httpCaller, d UploadData, conf Settings) (TUSResponse, error) {
	payload := UploadPayload{
		Name:        d.Filename,
		Description: d.VideoDescription,
		Password:    d.Password,
		Privacy: Privacy{
			Add:      conf.UploadSettings.Privacy.Add,
			Comments: conf.UploadSettings.Privacy.Comments,
			Embed:    conf.UploadSettings.Privacy.Embed,
			View:     conf.UploadSettings.Privacy.View,
			Download: conf.UploadSettings.Privacy.Download,
		},
		FolderURI:     "", // TODO
		ContentRating: []string{"safe"},
		Upload: UploadApproachSize{
			Approach: "tus",
			Size:     fmt.Sprint(d.FileSize),
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return TUSResponse{}, fmt.Errorf("unable to prepare video payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%v%v", uploadURI, uploadFilters), bytes.NewReader(bodyBytes))
	if err != nil {
		return TUSResponse{}, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/vnd.vimeo.*+json;version=3.4")
	req.Header.Add("Authorization", fmt.Sprintf("bearer %v", conf.PersonalAccessToken))

	retries := 0

	for retries < 2 {
		resp, err := c.Do(req)
		if err != nil {
			return TUSResponse{}, fmt.Errorf("error making post to vimeo upload URI: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			fmt.Println("rate limited, waiting for 60 seconds and trying again...")
			time.Sleep(time.Second * 60) // TODO: calculate time remaining
			retries++
			continue
		}

		// TODO: add specific status code handling and unmarshalling.
		if resp.StatusCode != http.StatusOK {
			respBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return TUSResponse{}, fmt.Errorf("could not read initiation response bytes: %v", err)
			}

			return TUSResponse{}, fmt.Errorf("received status code %v with response body: %v", resp.StatusCode, string(respBytes))
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

		return tResp, nil
	}

	return TUSResponse{}, errors.New("failed to upload three times, aborting...")
}

func getOffset(c httpCaller, tusURI string) (int64, error) {
	req, err := http.NewRequest(http.MethodHead, tusURI, nil)
	if err != nil {
		return -1, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Add("Tus-Resumable", "1.0.0")
	req.Header.Add("Accept", "application/vnd.vimeo.*+json;version=3.4")

	retries := 0

	for retries < 2 {
		resp, err := c.Do(req)
		if err != nil {
			return -1, fmt.Errorf("error making post to vimeo upload URI: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			fmt.Println("rate limited, waiting for 60 seconds and trying again...")
			time.Sleep(time.Second * 60) // TODO: calculate time remaining
			retries++
			continue
		}

		// TODO: add specific status code handling and unmarshalling.
		if resp.StatusCode != http.StatusOK {
			respBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return -1, fmt.Errorf("could not read initiation response bytes: %v", err)
			}

			return -1, fmt.Errorf("received status code %v with response body: %v", resp.StatusCode, string(respBytes))
		}

		// in this case we don't care what we get back, only about the Upload-Offset header value
		offsetStr := resp.Header.Get(UploadOffset)

		if offsetStr == "" {
			// retrying probably won't help, but try just in case
			retries++
			continue
		}

		offset, err := strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			return -1, fmt.Errorf("could not convert %v to a valid byte offset: %v", offsetStr, err)
		}

		return offset, nil
	}

	return -1, errors.New("unable to determine video offset")
}

func uploadFromOffset(c httpCaller, offset int64, tusURI, filePath string, chunkSizeMB int, fileSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file to upload: %v", err)
	}
	defer f.Close()

	fmt.Printf("Uploading %v....\n", f.Name())
	for offset < fileSize {
		var payloadSize int64 = int64(chunkSizeMB) * 1000000
		if (fileSize - offset) < payloadSize {
			payloadSize = fileSize - offset
		}
		fileBytes := make([]byte, payloadSize)

		_, err = f.ReadAt(fileBytes, offset)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("error reading file %v bytes at offset %v, file size %v: %v", filePath, offset, fileSize, err)
			}
		}

		req, err := http.NewRequest(http.MethodPatch, tusURI, bytes.NewReader(fileBytes))
		if err != nil {
			return fmt.Errorf("error creating request: %v", err)
		}

		req.Header.Add("Tus-Resumable", "1.0.0")
		req.Header.Add("Upload-Offset", fmt.Sprint(offset))
		req.Header.Add("Content-Type", "application/offset+octet-stream")

		retries := 0

		var newOffset int64 = -1
		for retries < 2 {
			resp, err := c.Do(req)
			if err != nil {
				return fmt.Errorf("error making post to vimeo upload URI: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				fmt.Println("rate limited, waiting for 60 seconds and trying again...")
				time.Sleep(time.Second * 60) // TODO: calculate time remaining
				retries++
				continue
			}

			// offset was set incorrectly, reset to what the header says it is and try again.
			if resp.StatusCode == http.StatusConflict {
				currOffsetStr := resp.Header.Get(UploadOffset)
				if currOffsetStr == "" {
					return fmt.Errorf("Received a 409 on upload but Upload-Offset header was empty for file %v, aborting...", filePath)
				}

				currOffset, err := strconv.ParseInt(currOffsetStr, 10, 64)
				if err != nil {
					return fmt.Errorf("could not convert %v to a valid byte offset: %v", currOffsetStr, err)
				}

				newOffset = currOffset
				break
			}

			// TODO: forbidden error unmarshalling and processing for better error messages - there are daily/weekly upload limits
			if resp.StatusCode != http.StatusNoContent {
				respBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					return fmt.Errorf("could not read upload response bytes: %v", err)
				}

				return fmt.Errorf("received error status code %v with response body: %v", resp.StatusCode, string(respBytes))
			}

			retOffsetStr := resp.Header.Get(UploadOffset)
			if retOffsetStr == "" {
				return fmt.Errorf("Upload-Offset header was empty for file %v, aborting...", filePath)
			}

			retOffset, err := strconv.ParseInt(retOffsetStr, 10, 64)
			if err != nil {
				return fmt.Errorf("could not convert %v to a valid byte offset: %v", retOffsetStr, err)
			}

			newOffset = retOffset
			break
		}

		// newOffset was not updated within 2 attempts, something went wrong
		if newOffset <= offset {
			return fmt.Errorf("unable to upload video portion after two attempts, please retry or troubleshoot, old offset was %v, new offset %v", offset, newOffset)
		}

		offset = newOffset
		percentUploaded := math.Floor((float64(newOffset) / float64(fileSize) * 100))

		fmt.Printf("%v%% uploaded...", percentUploaded)
	}

	fmt.Println()
	return nil
}
