package metadata

import (
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const (
	renameFileHint = "you may want to try renaming the file with a timestamp of when it was created at the start (ex. 2023-01-01 <filename>)"
)

// Class contains information about classes.
type Class struct {
	Name      string    `yaml:"name"`
	DayOfWeek string    `yaml:"day_of_week"`
	StartTime time.Time `yaml:"start_time"`
}

// CreationDateFromMDLS attempts to derive a file's creation date using the mdls command.
func CreationDateFromMDLS(absolutePath string) (time.Time, error) {

	metadata := exec.Command("mdls", absolutePath, "-name \"kMDItemContentCreationDate\"")
	awkCreationDate := exec.Command("awk", "'{print $3 \" \" $4}'")

	mOut, err := metadata.CombinedOutput()
	if err != nil {
		fmt.Printf("could not run mdls on %v: %v, skipping file...\n", absolutePath, err)
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
		fmt.Printf("error extracting %v creation date using awk: %v, skipping file...\n", absolutePath, err)
		fmt.Println(renameFileHint)
		return time.Now(), err
	}

	d, err := time.Parse("2006-01-02", string(creationDateBytes))
	if err != nil {
		fmt.Printf("error formatting %v creation date %v: %v, skipping file...\n", absolutePath, string(creationDateBytes), err)
		fmt.Println(renameFileHint)
		return time.Now(), err
	}

	//mdls returns UTC time; UTC to local offset will be 6 or 7 hours depending on DST.
	utcLocalOffset := time.Now().UTC().Sub(time.Now().Local())

	return d.Add(utcLocalOffset * -1), nil
}

// ClassNameWeek derives the semester, class name, and week of the semester it occurred on.
func ClassNameWeek(classes []Class, semesterStartDate time.Time, videoCreationDate time.Time) (string, error) {
	className := ""
	for _, c := range classes {
		if c.DayOfWeek != videoCreationDate.Weekday().String() {
			continue
		}

		// TODO: convert potential string time into something accurate

		fortyFiveMinsBeforeEnd := videoCreationDate.Add(time.Minute * 45)
		seventyFiveMinsAfterStart := videoCreationDate.Add(time.Minute * 75)

		if c.StartTime.Before(seventyFiveMinsAfterStart) && c.StartTime.After(fortyFiveMinsBeforeEnd) {
			className = c.Name
			break
		}
	}

	if className == "" {
		fmt.Printf("could not determine class name based on file creation date\n")
		fmt.Println(renameFileHint)
		return "", errors.New("failed to determine class name from file creation date")
	}

	// 168 hours per week
	weekNumber := time.Since(semesterStartDate).Hours() / 168

	season := yearSeason(semesterStartDate)

	// ex. Advanced Tap 2023 Spring - Week 10; season and year added to make video names unique
	return fmt.Sprintf("%v %v - Week %v", className, season, weekNumber), nil
}

// yearSeason returns the year and season of the given date.
func yearSeason(d time.Time) string {
	year := d.Year()
	season := ""

	switch d.Month() {
	case time.January, time.February, time.March, time.April, time.May:
		season = "Spring"
	case time.June, time.July, time.August, time.September:
		season = "Summer"
	case time.October, time.November, time.December:
		season = "Winter"
	}

	return fmt.Sprintf("%v %v", year, season)
}
