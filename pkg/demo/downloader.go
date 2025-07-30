package demo

import (
	"compress/bzip2"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// progressReader wraps an io.Reader to track download progress
type progressReader struct {
	reader       io.Reader
	totalBytes   int64
	readBytes    int64
	lastProgress int
	lastUpdate   time.Time
}

func newProgressReader(reader io.Reader, totalBytes int64) *progressReader {
	return &progressReader{
		reader:     reader,
		totalBytes: totalBytes,
		lastUpdate: time.Now(),
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.readBytes += int64(n)

	// Update progress at most 10 times per second
	if time.Since(pr.lastUpdate) >= 100*time.Millisecond {
		progress := 0
		if pr.totalBytes > 0 {
			progress = int(float64(pr.readBytes) / float64(pr.totalBytes) * 100)
		}

		if progress != pr.lastProgress {
			fmt.Printf("\rDownloading: %d%% complete", progress)
			if progress == 100 {
				fmt.Println()
			}
			pr.lastProgress = progress
			pr.lastUpdate = time.Now()
		}
	}

	return n, err
}

// DownloadDemo downloads a demo file from a share code and returns the path to the downloaded file
func DownloadFromShareCode(shareCode string, outputDir string) (string, error) {
	// Get the download URL from the share code
	url := ReplayURL(shareCode)

	// Create a temporary directory if none specified
	if outputDir == "" {
		var err error
		outputDir, err = os.MkdirTemp("", "cs2-demos")
		if err != nil {
			return "", fmt.Errorf("failed to create temp directory: %w", err)
		}
	} else {
		// Make sure the output directory exists
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Create output file name based on the share code
	fileName := filepath.Join(outputDir, shareCode+".dem")

	// Download and decompress the file
	if err := downloadAndDecompress(url, fileName); err != nil {
		return "", err
	}

	return fileName, nil
}

// downloadAndDecompress downloads a bz2 compressed file and decompresses it
func downloadAndDecompress(url string, outputPath string) error {
	// Download the file
	fmt.Printf("Downloading demo from: %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create the output file
	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer output.Close()

	// Create a progress reader
	progressReader := newProgressReader(resp.Body, resp.ContentLength)

	// Decompress bzip2 data
	fmt.Println("Decompressing demo file...")
	bz2Reader := bzip2.NewReader(progressReader)

	// Copy decompressed data to output file
	_, err = io.Copy(output, bz2Reader)
	if err != nil {
		return fmt.Errorf("failed to decompress and write file: %w", err)
	}

	fmt.Println("Download and decompression complete!")
	return nil
}
