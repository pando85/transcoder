package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"

	"github.com/spf13/pflag"
	"golift.io/starr"
	"golift.io/starr/radarr"
)

type MovieBySize []*radarr.Movie

func (s MovieBySize) Len() int           { return len(s) }
func (s MovieBySize) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s MovieBySize) Less(i, j int) bool { return getSize(s[i]) > getSize(s[j]) }

func getSize(m *radarr.Movie) int64 {
	if m.MovieFile != nil {
		return m.MovieFile.Size
	}
	return 0
}

func isNotX265OrH265(m *radarr.Movie) bool {
	return m.MovieFile != nil && m.MovieFile.MediaInfo != nil &&
		(m.MovieFile.MediaInfo.VideoCodec != "x265" && m.MovieFile.MediaInfo.VideoCodec != "h265")
}

func HumanReadableSize(size int64) string {
	if size == 0 {
		return "N/A"
	}

	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

type ScheduledItem struct {
	SourcePath      string      `json:"sourcePath"`
	DestinationPath string      `json:"destinationPath"`
	ID              string      `json:"id"`
	Events          interface{} `json:"events"`
}

type FailedItem struct {
	SourcePath      string `json:"sourcePath"`
	DestinationPath string `json:"destinationPath"`
	ForceCompleted  bool   `json:"forceCompleted"`
	ForceFailed     bool   `json:"forceFailed"`
	ForceExecuting  bool   `json:"forceExecuting"`
	ForceAdded      bool   `json:"forceAdded"`
	Priority        int    `json:"priority"`
	Error           string `json:"error"`
}

type Response struct {
	Scheduled []ScheduledItem `json:"scheduled"`
	Failed    []FailedItem    `json:"failed"`
	Skipped   interface{}     `json:"skipped"`
}

func PrintTranscoderResponse(jsonStr []byte) error {
	var response Response
	if err := json.Unmarshal(jsonStr, &response); err != nil {
		return err
	}

	switch {
	case len(response.Scheduled) > 0:
		fmt.Println("Movie successfully added.")
	case len(response.Failed) > 0:
		fmt.Println("Movie was not added.")
	default:
		return errors.New("Movie was neither added nor failed.")
	}

	return nil
}

func AddMovieToTranscoderQueue(path string, url string, token string) error {
	payload := map[string]string{
		"SourcePath": path,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadJSON))
	if err != nil {
		return err
	}

	authHeader := fmt.Sprintf("Bearer %s", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	client := http.Client{}

	fmt.Println("Adding movie to transcoder queue")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = PrintTranscoderResponse(body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Failed with status %s and message: %s", resp.Status, body)
	}

	fmt.Println()
	fmt.Println()
	return nil
}

func main() {
	apiKey := pflag.StringP("api-key", "k", "", "Radarr API key")
	radarrURL := pflag.StringP("url", "u", "", "Radarr server URL")
	numMovies := pflag.Int("movies", 10, "Number of movies to retrieve")
	transcoderURL := pflag.String("transcoder-url", "", "Transcoder server URL")
	transcoderToken := pflag.String("transcoder-token", "", "Transcoder web server token")
	dryRun := pflag.Bool("dry-run", false, "Dry run mode doesn't add movies to transcoder queue")

	pflag.Parse()

	if *apiKey == "" || *radarrURL == "" || *transcoderURL == "" || *transcoderToken == "" {
		fmt.Println("Both API key and Radarr URL are required.")
		pflag.PrintDefaults()
		return
	}

	transcoderPostURL := fmt.Sprintf("%s/api/v1/job/", *transcoderURL)

	c := starr.New(*apiKey, *radarrURL, 0)
	r := radarr.New(c)

	movies, err := r.GetMovie(0)
	if err != nil {
		panic(err)
	}

	var filteredMovies MovieBySize
	for _, m := range movies {
		if isNotX265OrH265(m) {
			filteredMovies = append(filteredMovies, m)
		}
	}

	sort.Sort(filteredMovies)

	fmt.Printf("Number of filtered movies: %d\n", len(filteredMovies))

	for i, m := range filteredMovies {
		if i >= *numMovies {
			break
		}

		fmt.Printf("Title: %s\n", m.Title)
		fmt.Printf("File Path: %s\n", m.Path)

		if m.MovieFile != nil {
			fmt.Printf("Codec: %s\n", m.MovieFile.MediaInfo.VideoCodec)

			fmt.Printf("Size: %s\n", HumanReadableSize(getSize(m)))
			fmt.Printf("Full Path: %s\n\n", m.MovieFile.Path)

			if !*dryRun {
				err := AddMovieToTranscoderQueue(m.MovieFile.Path, transcoderPostURL, *transcoderToken)
				if err != nil {
					fmt.Println("error:", err)
					os.Exit(1)
				}
			}
		}
	}
}
