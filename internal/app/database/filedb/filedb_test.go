package filedb_test

import (
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/nmalensek/video-uploader/internal/app/database"
	"github.com/nmalensek/video-uploader/internal/app/database/filedb"
)

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
			defer removeTestFile()
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
	defer removeTestFile()
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

func TestFileDB_GetPutEndToEnd(t *testing.T) {
	defer removeTestFile()
	fdb, err := filedb.New(".")
	if err != nil {
		t.Fatal(err)
	}

	r, err := fdb.GetUpload("test item 1")
	if err != nil {
		t.Fatal(err)
	}

	if !r.IsEmpty() {
		t.Fatalf("TestFileDB_GetPutEndToEnd() expected initial get to be empty, was: %+v", r)
	}

	testItemOne := database.UploadRecord{
		Name:           "test item 1",
		CalculatedName: "test item 1",
		TusURI:         "https://test.com",
		VideoURI:       "/videos/1234",
		Status:         database.InProgress,
		ErrorDetails:   nil,
	}

	err = fdb.PutUpload(testItemOne)
	if err != nil {
		t.Fatal(err)
	}

	item, err := fdb.GetUpload("test item 1")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(testItemOne, item); diff != "" {
		t.Errorf("TestFileDB_GetPutEndToEnd() mismatch (-want +got):\n%s", diff)
		return
	}

	// put a second item
	testItemTwo := database.UploadRecord{
		Name:           "test item 2",
		CalculatedName: "test item 2",
		TusURI:         "https://test.com",
		VideoURI:       "/videos/1234",
		Status:         database.InProgress,
		ErrorDetails:   nil,
	}

	err = fdb.PutUpload(testItemTwo)
	if err != nil {
		t.Fatal(err)
	}

	item2, err := fdb.GetUpload("test item 2")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(testItemTwo, item2); diff != "" {
		t.Errorf("TestFileDB_GetPutEndToEnd() mismatch (-want +got):\n%s", diff)
		return
	}

	// update existing item's status, simulating an upload finishing.
	updatedTestItemOne := database.UploadRecord{
		Name:           "test item 1",
		CalculatedName: "test item 1", // shouldn't actually change normally
		TusURI:         "https://test.com",
		VideoURI:       "/videos/1234",
		Status:         database.Complete,
		ErrorDetails:   nil,
	}

	err = fdb.PutUpload(updatedTestItemOne)
	if err != nil {
		t.Fatal(err)
	}

	itemOneUpdated, err := fdb.GetUpload("test item 1")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(updatedTestItemOne, itemOneUpdated); diff != "" {
		t.Errorf("TestFileDB_GetPutEndToEnd() mismatch (-want +got):\n%s", diff)
		return
	}

}
