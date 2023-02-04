package filedb

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nmalensek/video-uploader/internal/app/database"
)

type FileDB struct {
	uploadsFile string
}

const (
	uploadsFilename = "uploads.json"
)

func New(outputFolder string) (FileDB, error) {
	_, err := os.ReadDir(outputFolder)
	if err != nil {
		return FileDB{}, fmt.Errorf("could not open output folder %v: %v", outputFolder, err)
	}

	if !strings.HasSuffix(outputFolder, "/") {
		outputFolder = outputFolder + "/"
	}

	return FileDB{
		uploadsFile: fmt.Sprintf("%v%v", outputFolder, uploadsFilename),
	}, nil
}

// GetUpload reads the uploadsFile and gets the record if it exists or returns an empty UploadRecord.
func (f FileDB) GetUpload(key string) (database.UploadRecord, error) {
	file, err := os.Open(f.uploadsFile)
	if err != nil {
		return database.UploadRecord{}, fmt.Errorf("error opening uploads file: %v", err)
	}
	defer file.Close()

	// bad practice: read in the whole file. however, file should only grow by ~200kb max per year if a new
	// file is not generated per year.
	bytes, err := io.ReadAll(file)
	if err != nil {
		return database.UploadRecord{}, fmt.Errorf("error reading uploads file: %v", err)
	}

	var uploadRecords map[string]database.UploadRecord
	err = json.Unmarshal(bytes, &uploadRecords)
	if err != nil {
		return database.UploadRecord{}, fmt.Errorf("error unmarshaling uploads file: %v", err)
	}

	return uploadRecords[key], nil
}

// PutUpload writes the given UploadRecord to the uploadFile, overwriting the current item if it exists.
func (f FileDB) PutUpload(item database.UploadRecord) error {
	if item.Name == "" {
		return fmt.Errorf("cannnot save item %+v, name is empty", item)
	}

	file, err := os.Open(f.uploadsFile)
	if err != nil {
		return fmt.Errorf("error opening uploads file: %v", err)
	}
	defer file.Close()

	// bad practice: read in the whole file. however, file should only grow by ~200kb max per year if a new
	// file is not generated per year.
	bytes, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error reading uploads file: %v", err)
	}

	var uploadRecords map[string]database.UploadRecord
	err = json.Unmarshal(bytes, &uploadRecords)
	if err != nil {
		return fmt.Errorf("error unmarshaling uploads file: %v", err)
	}

	uploadRecords[item.Name] = item

	newBytes, err := json.Marshal(&uploadRecords)
	if err != nil {
		return fmt.Errorf("error marshaling uploads data: %v", err)
	}

	err = os.WriteFile(f.uploadsFile, newBytes, 0666)
	if err != nil {
		return fmt.Errorf("error writing updated upload records: %v", err)
	}

	return nil
}
