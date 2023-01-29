package passphrase_test

import (
	"strings"
	"testing"

	"github.com/nmalensek/video-uploader/internal/app/passphrase"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name      string
		numChunks int
		wantErr   bool
	}{
		{
			name:      "four word passphrase",
			numChunks: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := passphrase.Generate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(strings.Split(got, "_")) != tt.numChunks {
				t.Errorf("Generate() = %v is %v chunks, want %v chunks", got,
					len(strings.Split(got, "_")), tt.numChunks)
			}
		})
	}
}
