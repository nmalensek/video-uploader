package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nmalensek/vimeo-uploader/internal/app/passphrase"
	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "", "the absolute path to the config file in YAML format. If empty, checks the folder the executable is launched from for a file named config.yaml.")
)

const (
	renameFileHint = "you may want to try renaming the file with a timestamp of when it was created at the start (ex. 2023-01-01 <filename>)"
)

type uploadConfig struct {
	SemesterStartDate  time.Time     `yaml:"semester_start_date"`
	UploadFolderPath   string        `yaml:"upload_folder_path"`
	FinishedFolderPath string        `yaml:"finished_folder_path"`
	ChunkSizeMB        int           `yaml:"chunk_size_mb"`
	LogLevel           string        `yaml:"log_level"`
	VimeoSettings      vimeoSettings `yaml:"vimeo_settings"`
	Classes            []class       `yaml:"classes"`
}

type vimeoSettings struct {
	PersonalAccessToken string         `yaml:"personal_access_token"`
	UploadSettings      uploadSettings `yaml:"upload_settings"`
}

type uploadSettings struct {
	ContentRating string  `yaml:"content_rating"`
	Privacy       privacy `yaml:"privacy"`
}

type privacy struct {
	Comments string `yaml:"comments"`
	Embed    string `yaml:"embed"`
	View     string `yaml:"view"`
}

type class struct {
	Name      string    `yaml:"name"`
	DayOfWeek string    `yaml:"day_of_week"`
	StartTime time.Time `yaml:"start_time"`
}

type uploader interface {
	Upload(fileName string, filePath string, chunkSize int) error
}

func main() {
	cfg := readConfig()

	processFiles(cfg)
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

func processFiles(conf uploadConfig) {
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
			t, err := creationDateFromMDLS(file.Name())
			if err != nil {
				// error messages printed in called function, skip file since both methods failed.
				continue
			}

			fileCreationDate = t
		} else {
			fileCreationDate = d
		}

		fileName, err := classNameWeek(conf, fileCreationDate)
		if err != nil {
			// error messages printed in called function, skip file since which class it is is unknown.
			continue
		}

		password, pErr := passphrase.Generate()
		if pErr != nil {
			fmt.Printf("error generating random password: %v, skipping file...\n", err)
			continue
		}

		// upload

	}
}

func creationDateFromMDLS(filename string) (time.Time, error) {

	// TODO: validate command, if needed prepend path to name
	metadata := exec.Command("mdls", filename, "-name \"kMDItemContentCreationDate\"")
	awkCreationDate := exec.Command("awk", "'{print $3 \" \" $4}'")

	mOut, err := metadata.CombinedOutput()
	if err != nil {
		fmt.Printf("could not run mdls on %v: %v, skipping file...\n", filename, err)
		fmt.Println(renameFileHint)
		return time.Now(), err
	}

	awkIn, err := awkCreationDate.StdinPipe()
	if err != nil {
		fmt.Printf("error getting stdin pipe for awk command: %v, skipping file...\n", err)
		fmt.Println(renameFileHint)
		return time.Now(), err
	}

	awkIn.Write(mOut)
	awkIn.Close()

	creationDateBytes, err := awkCreationDate.CombinedOutput()
	if err != nil {
		fmt.Printf("error extracting %v creation date using awk: %v, skipping file...\n", filename, err)
		fmt.Println(renameFileHint)
		return time.Now(), err
	}

	d, err := time.Parse("2006-01-02", string(creationDateBytes))
	if err != nil {
		fmt.Printf("error formatting %v creation date %v: %v, skipping file...\n", filename, string(creationDateBytes), err)
		fmt.Println(renameFileHint)
		return time.Now(), err
	}

	return d, nil
}

func classNameWeek(conf uploadConfig, date time.Time) (string, error) {
	className := ""
	for _, c := range conf.Classes {
		if c.DayOfWeek != date.Weekday().String() {
			continue
		}

		// TODO: convert potential string time into something accurate

		fifteenMinsBefore := date.Add(time.Minute * -15)
		thirtyMinsAfter := date.Add(time.Minute * 30)

		if c.StartTime.Before(thirtyMinsAfter) && c.StartTime.After(fifteenMinsBefore) {
			className = c.Name
			break
		}
	}

	if className == "" {
		fmt.Printf("could not determine class name based on file creation date, skipping file...\n")
		fmt.Println(renameFileHint)
		return "", errors.New("failed to determine class name from file creation date")
	}

	// 168 hours per week
	weekNumber := time.Since(conf.SemesterStartDate).Hours() / 168

	return fmt.Sprintf("%v - Week %v", className, weekNumber), nil
}

// description
// name
// password
// upload.approach
// upload.size

// for each .mp4 file
//  based on date, get class name from config
//  calculate weeks since start
//      that's the name
//  generate password
//  get file size
//  post to upload endpoint with derived name + week, standard settings, password, and length
//      returns upload URI
//      what status codes can this return? conflict? unauth?
//  open file and stream in chunk size chunks to returned upload URI
//  log progress per chunk (verbose)
//  log when finished with name, password
//  write out filename of successful upload to .txt file
//  move file to 'uploaded' folder
