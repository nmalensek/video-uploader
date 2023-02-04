package database

// UploadDatastore contains access patterns for upload datastores.
type UploadDatastore interface {
	GetUpload(key string) (UploadRecord, error)
	PutUpload(item UploadRecord) error
}

// UploadRecord is information about the status of a file upload attempt and the errors
// that occurred, if any. If an upload fails but its tus URI is populated, the upload may be resumable
// depending on upload implementation. If an error occurred, the status will be set correspondingly
// and contain details about the error.
type UploadRecord struct {
	Name           string       `json:"name"`
	CalculatedName string       `json:"calculated_name"`
	TusURI         string       `json:"tus_uri"`
	VideoURI       string       `json:"video_uri"`
	Status         UploadStatus `json:"status"`
	ErrorDetails   error        `json:"errorDetails,omitempty"`
}

// IsEmpty checks relevant UploadRecord properties and returns whether it contains data.
// Name is used as the key so it must not be empty if the record exists.
func (u UploadRecord) IsEmpty() bool {
	if u.Name != "" {
		return false
	}

	return true
}

// UploadStatus is a string representing the state of an UploadRecord. Helps determine whether an upload was completed successfully.
type UploadStatus string

const (
	Complete   UploadStatus = "COMPLETE"
	InProgress UploadStatus = "IN_PROGRESS"
	Error      UploadStatus = "ERROR"
)
