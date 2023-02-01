package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nmalensek/video-uploader/internal/app/metadata"
	"github.com/nmalensek/video-uploader/internal/app/passphrase"
	"github.com/nmalensek/video-uploader/internal/app/vimeo"
	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "", "the absolute path to the config file in YAML format. If empty, checks the folder the executable is launched from for a file named config.yaml.")
)

type uploadConfig struct {
	SemesterStartDate  time.Time        `yaml:"semester_start_date"`
	UploadFolderPath   string           `yaml:"upload_folder_path"`
	FinishedFolderPath string           `yaml:"finished_folder_path"`
	VideoStatusPath    string           `yaml:"upload_status_path"`
	ChunkSizeMB        int              `yaml:"chunk_size_mb"`
	LogLevel           string           `yaml:"log_level"`
	VimeoSettings      vimeo.Settings   `yaml:"vimeo_settings"`
	Classes            []metadata.Class `yaml:"classes"`
}

type uploader interface {
	Upload(data vimeo.UploadData) error
}

func main() {
	cfg := readConfig()

	cl := &http.Client{
		Timeout: time.Second * 10,
	}

	vimeoUploader, err := vimeo.NewUploader(cfg.VideoStatusPath, cl, cfg.VimeoSettings)
	if err != nil {
		log.Fatal(err)
	}

	processFiles(cfg, vimeoUploader)
}

func readConfig() uploadConfig {
	flag.Parse()

	if *configPath == "" {
		ex, err := os.Executable()
		if err != nil {
			log.Fatalf("could not determine executable path: %v", err)
		}

		path := fmt.Sprintf("%v/%v", filepath.Dir(ex), "config.yaml")
		configPath = &path
	}

	file, err := os.Open(*configPath)
	if err != nil {
		log.Fatalf("could not open config file: %v", err)
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("could not read config file: %v", err)
	}

	var conf uploadConfig
	yaml.Unmarshal(fileBytes, &conf)
	if err != nil {
		log.Fatalf("could not unmarshal config file: %v", err)
	}

	return conf
}

func processFiles(conf uploadConfig, uploadClient uploader) {
	files, err := os.ReadDir(conf.UploadFolderPath)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".mp4") {
			continue
		}

		i, err := file.Info()
		if err != nil {
			fmt.Printf("error occurred getting %v info: %v. skipping file...\n", file.Name(), err)
			continue
		}

		var fileCreationDate time.Time

		nameChunks := strings.Split(file.Name(), " ")

		d, pErr := time.Parse("2006-01-02", nameChunks[0])
		if pErr != nil {
			fmt.Printf("unable to get creation date from name, falling back to mdls...\n")

			// fallback if filename is not prefixed with timestamp
			t, err := metadata.CreationDateFromMDLS(file.Name())
			if err != nil {
				// error messages printed in called function, skip file since both methods failed.
				continue
			}

			fileCreationDate = t
		} else {
			fileCreationDate = d
		}

		fileName, err := metadata.ClassNameWeek(conf.Classes, conf.SemesterStartDate, fileCreationDate)
		if err != nil {
			// error messages printed in called function, skip file since which class it is is unknown.
			continue
		}

		password, pErr := passphrase.Generate()
		if pErr != nil {
			fmt.Printf("error generating random password: %v, skipping file...\n", err)
			continue
		}

		uErr := uploadClient.Upload(vimeo.UploadData{
			Filename:  fileName,
			FilePath:  fmt.Sprintf("%v/%v", conf.UploadFolderPath, file.Name()),
			Password:  password,
			FileSize:  i.Size(),
			ChunkSize: conf.ChunkSizeMB,
		})

		if uErr != nil {
			fmt.Printf("error uploading %v, file may need to be re-processed. skipping...\n", file.Name())
		}
	}
}

//  post to upload endpoint with derived name + week, standard settings, password, and length
//      returns upload URI
// 		if ok status code, save video name and upload URI to .json "database" file with status "IN_PROGRESS"
//      what status codes can this return? conflict? unauth? If an error response occurs, set status "ERROR" and fill out error details in the file
//  open file and stream in chunk size chunks to returned upload URI
//  log progress per chunk (verbose)
//  log when finished with name, password
//  update video entry status in "database" to "COMPLETE"
//  move file to 'uploaded' folder
