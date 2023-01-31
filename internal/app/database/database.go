package database

// UploadDatastore contains access patterns for upload datastores.
type UploadDatastore interface {
	GetUpload(key string) (UploadRecord, error)
	PutUpload(item UploadRecord) error
}

// UploadRecord is information about the status of a file upload attempt and the errors
// that occurred, if any. If an upload fails but its URI is populated, the upload may be resumable
// depending on upload implementation. If an error occurred, the status will be set correspondingly
// and contain details about the error.
type UploadRecord struct {
	Name         string       `json:"name"`
	URI          string       `json:"uri"`
	Status       UploadStatus `json:"status"`
	ErrorDetails error        `json:"errorDetails,omitempty"`
}

type UploadStatus string

const (
	Complete   UploadStatus = "COMPLETE"
	InProgress UploadStatus = "IN_PROGRESS"
	Error      UploadStatus = "ERROR"
)
