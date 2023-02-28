package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
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

	uploadCl := &http.Client{
		Timeout: time.Minute * 20,
	}

	vimeoUploader, err := vimeo.NewUploader(cfg.VideoStatusPath, cl, uploadCl, cfg.VimeoSettings)
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
	defer file.Close()

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

		if !strings.HasSuffix(file.Name(), ".mov") && !strings.HasSuffix(file.Name(), ".mp4") {
			continue
		}

		i, err := file.Info()
		if err != nil {
			fmt.Printf("error occurred getting %v info: %v. skipping file...\n", file.Name(), err)
			continue
		}

		// currently only using it for metrics, file name is expected to be final video name.
		// temporarily skip this until this can be worked out reliably.
		// calculatedFileName, _ := getVideoNameByDate(file, conf.UploadFolderPath, conf.Classes, conf.SemesterStartDate)

		password, pErr := passphrase.Generate()
		if pErr != nil {
			fmt.Printf("error generating random password: %v, skipping file...\n", err)
			continue
		}

		uErr := uploadClient.Upload(vimeo.UploadData{
			Filename:         file.Name(),
			VideoDescription: strings.TrimSuffix(file.Name(), ".mp4"),
			VideoName:        "",
			FilePath:         fmt.Sprintf("%v/%v", conf.UploadFolderPath, file.Name()),
			Password:         password,
			FileSize:         i.Size(),
			ChunkSize:        conf.ChunkSizeMB,
		})

		if uErr != nil {
			fmt.Printf("error uploading %v, file may need to be re-processed. error: %v\n skipping...\n", file.Name(), uErr)
		}

		// os.MkdirAll(fmt.Sprintf("%v/%v", conf.FinishedFolderPath, "uploaded"), 0750)

		// rErr := os.Rename(fmt.Sprintf("%v/%v", conf.UploadFolderPath, file.Name()), fmt.Sprintf("%v/%v/%v", conf.FinishedFolderPath, "uploaded", file.Name()))
		// if rErr != nil {
		// 	fmt.Printf("could not move file %v into completed uploads folder: %v", file.Name(), err)
		// }
	}
}

func getVideoNameByDate(file fs.DirEntry, fileDir string, classes []metadata.Class, startDate time.Time) (string, error) {
	nameChunks := strings.Split(file.Name(), " ")

	var fileCreationDate time.Time

	d, pErr := time.Parse("2006-01-02T15:04:05Z", nameChunks[0])
	if pErr != nil {
		fmt.Printf("unable to get creation date from name, falling back to mdls...\n")

		// fallback if filename is not prefixed with timestamp
		t, err := metadata.CreationDateFromMDLS(fmt.Sprintf("%v/%v", fileDir, file.Name()))
		if err != nil {
			// error messages printed in called function.
			return "", err
		}

		fileCreationDate = t
	} else {
		fileCreationDate = d
	}

	calculatedFileName, err := metadata.ClassNameWeek(classes, startDate, fileCreationDate)
	if err != nil {
		// error messages printed in called function, skip file since which class it is is unknown.
		return "", err
	}

	return calculatedFileName, nil
}
