package main

import (
	"os"
	"os/exec"
	"fmt"
	"log"
	"bytes"
	"errors"
	"strings"
	"crypto/rand"
	"path/filepath"
	"encoding/json"
	"encoding/base64"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(filename, mediaType string) string {
	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", filename, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func (cfg apiConfig) getS3URL(prefix, key string) string {
	// 'key' being the object filename
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/%s", cfg.s3Bucket, cfg.s3Region, prefix, key)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func makeFilename() (newFilename string, err error) {
	randFilenameBytes := make([]byte, 32)
	_, err = rand.Read(randFilenameBytes)
	if err != nil {
		return "", err
	}
	randFilenameString := base64.RawURLEncoding.EncodeToString(randFilenameBytes)
	return randFilenameString, nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmdToRun := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var cmdBuffer bytes.Buffer
	var errBuffer bytes.Buffer
	cmdToRun.Stdout = &cmdBuffer
	cmdToRun.Stderr = &errBuffer
	err := cmdToRun.Run()
	if err != nil {
		return "", fmt.Errorf("ffprobe failed: %w: %s", err, errBuffer.String())
	}

	type videoOrientation struct {
		Width		int		`json:"width"`
		Height		int 	`json:"height"`
		CodecType	string	`json:"codec_type"`
	}

	type ffprobeStreams struct {
		Streams	[]videoOrientation `json:"streams"`
	}
	var ffprobeResults ffprobeStreams

	if err := json.Unmarshal(cmdBuffer.Bytes(), &ffprobeResults); err != nil {
		log.Print("Error unmarshalling JSON")
		return "", err
	}
	if len(ffprobeResults.Streams) == 0 {
		log.Print("No streams acquired via ffprobe command")
		return "", errors.New("Input could not be parsed")
	}
	var readFromStream videoOrientation
	bFoundVideoStream := false
	for _, stream := range ffprobeResults.Streams {
		if stream.CodecType == "video" {
			readFromStream = stream
			bFoundVideoStream = true
			break
		}
	}
	if !bFoundVideoStream {
		log.Printf("Error reading from %s: no video stream found", filePath)
		return "", errors.New("No video stream found in input")
	}

	dimensions := videoOrientation{
		Width:	readFromStream.Width,
		Height:	readFromStream.Height,
	}
	
	aspectRatio := getAspectRatioEstimate(dimensions.Width, dimensions.Height)
	
	if aspectRatio == "16:9" || aspectRatio == "9:16" {
		return aspectRatio, nil
	} else {
		//log.Printf("Recording as 'other' because aspect ratio is: %s", aspectRatio)
		return "other", nil
	}
}

func getAspectRatioEstimate(width, height int) string {
	if width > height {
    return "16:9"
	} else if width < height {
		return "9:16"
	} else {
		return "other" // square
	}
}

func getGreatestCommonDenominator(a int, b int) int {
	// Can be used for stricter logic. Getting the aspect ratio would look like:
	//gcd := getGreatestCommonDenominator(dimensions.Width, dimensions.Height)
	//aspectRatio := fmt.Sprintf("%d:%d", dimensions.Width/gcd, dimensions.Height/gcd)
	//log.Printf("Calculated aspect ratio: %s (width: %d, height: %d, gcd: %d)", aspectRatio, dimensions.Width, dimensions.Height, gcd)

	if a <= 0 { if b <= 0 { return 1 } else { return b } }
	if b <= 0 { return a }
	for b != 0 {
		remainder := a % b
		a = b
		b = remainder
	}
	if a <= 0 { return 1 } else { return a }
}

func getLowestCommonDenominator(a int, b int) int {
	g := max(a, b)
	s := min(a, b);

	for i := g; i <= a * b; i += g {
		if (i % s == 0) {
			return i;
		}
	}

	return 1
}