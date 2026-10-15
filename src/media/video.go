package media

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/u2takey/ffmpeg-go"
)

// VideoThumbnail generates a thumbnail image from a video at a specific frame.
func VideoThumbnail(content []byte, frameNum int, size struct{ Width int }) ([]byte, error) {
	// ffmpeg needs a seekable input: MP4 files with the moov atom at the end can't be read from a pipe
	input, err := os.CreateTemp("", "gows-video-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(input.Name())
	_, err = input.Write(content)
	closeErr := input.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", closeErr)
	}

	var buf bytes.Buffer
	var stderr bytes.Buffer
	cmd := ffmpeg_go.
		Input(input.Name()).
		Filter("scale", ffmpeg_go.Args{fmt.Sprintf("%d:-1", size.Width)}).
		Filter("select", ffmpeg_go.Args{fmt.Sprintf("gte(n,%d)", frameNum)}).
		Output("pipe:", ffmpeg_go.KwArgs{"vframes": 1, "format": "image2pipe"}).
		GlobalArgs("-hide_banner", "-loglevel", "error").
		WithOutput(&buf).
		WithErrorOutput(&stderr).
		Silent(true).
		OverWriteOutput()
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	data := buf.Bytes()
	if len(data) == 0 {
		return nil, fmt.Errorf("no thumbnail data returned")
	}
	return data, nil
}
