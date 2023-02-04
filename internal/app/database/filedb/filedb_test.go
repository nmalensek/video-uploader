package filedb_test

import (
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/nmalensek/video-uploader/internal/app/database"
	"github.com/nmalensek/video-uploader/internal/app/database/filedb"
)

func TestMain(m *testing.M) {
	code := m.Run()
	removeTestFile()
	os.Exit(code)
}

func removeTestFile() {
	err := os.Remove("./uploads.json")
	if err != nil {
		log.Fatal("unable to delete test uploads file")
	}
}

func TestFileDB_GetUpload(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		want    database.UploadRecord
		wantErr bool
	}{
		{
			name:    "able to get empty record when it doesn't exist",
			key:     "doesnt_exist",
			want:    database.UploadRecord{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := filedb.New(".")
			if err != nil {
				t.Fatal(err)
			}

			got, err := f.GetUpload(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileDB.GetUpload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FileDB.GetUpload() = %v, want %v", got, tt.want)
				return
			}

			if got.IsEmpty() != true {
				t.Errorf("FileDB.GetUpload() IsEmpty = %v, want true", got.IsEmpty())
			}
		})
	}
}

func TestFileDB_PutUpload(t *testing.T) {
	tests := []struct {
		name    string
		item    database.UploadRecord
		wantErr bool
	}{
		{
			name: "add new item to database",
			item: database.UploadRecord{
				Name:         "test item 1",
				TusURI:       "https://test.com",
				VideoURI:     "/video/1234",
				Status:       database.InProgress,
				ErrorDetails: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := filedb.New(".")
			if err != nil {
				t.Fatal(err)
			}

			if err := f.PutUpload(tt.item); (err != nil) != tt.wantErr {
				t.Errorf("FileDB.PutUpload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
