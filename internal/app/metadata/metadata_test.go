package metadata_test

import (
	"testing"
	"time"
)

func TestCalculateUTCOffset(t *testing.T) {
	d, _ := time.Parse("2006-01-05", "2023-01-29T23:00:00")

	utcLocalOffset := time.Now().UTC().Sub(time.Now().Local())

	got := d.Add(utcLocalOffset * -1)

	want, _ := time.Parse("2006-01-05", "2023-01-29T16:00:00")

	if got != want {
		t.Errorf("TestCalculateUTCOffset() = got %v want %v", got, want)
	}
}
