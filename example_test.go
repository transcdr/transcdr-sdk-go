package transcdr_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	transcdr "github.com/transcdr/transcdr-sdk-go"
)

func Example() {
	ctx := context.Background()
	client := transcdr.NewClient(transcdr.WithAPIKey(os.Getenv("TRANSCDR_API_KEY")))

	job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{
		Input:  transcdr.URLInput("https://example.com/talk.mov"),
		Preset: transcdr.String("hls-av1-abr"),
	})
	if err != nil {
		log.Fatal(err)
	}
	job, err = client.Jobs.WaitFor(ctx, job.ID, &transcdr.WaitForOptions{Timeout: 30 * time.Minute})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(job.Status, *job.PlaybackURL)
}

func ExampleUploadsService_UploadPath() {
	ctx := context.Background()
	client := transcdr.NewClient()

	asset, err := client.Uploads.UploadPath(ctx, "talk.mov", &transcdr.UploadFileOptions{
		OnProgress: func(p transcdr.UploadProgress) { fmt.Printf("\r%.0f %%", p.Percent) },
	})
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.Jobs.Create(ctx, &transcdr.JobCreateParams{Input: transcdr.AssetInput(asset.ID), Preset: transcdr.String("web-av1-1080p")})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleJobsService_All() {
	ctx := context.Background()
	client := transcdr.NewClient()

	for job, err := range client.Jobs.All(ctx, &transcdr.JobListParams{Status: transcdr.JobFailed}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(job.ID, job.Error.Code)
	}
}

func ExamplePresetsService_Create() {
	ctx := context.Background()
	client := transcdr.NewClient()

	// Constant bit rate HLS: each rendition at its own rate, else quality.bitrate.
	preset, err := client.Presets.Create(ctx, &transcdr.PresetCreateParams{
		Name: "Broadcast CBR",
		Output: &transcdr.OutputSpecInput{
			Mode:    "hls",
			Codec:   "h264",
			Quality: &transcdr.Quality{Target: transcdr.String(transcdr.QualityCBR), Bitrate: transcdr.String("3M")},
			Renditions: []transcdr.Rendition{
				{Width: 1920, Height: 1080, Bitrate: transcdr.String("6M")},
				{Width: 1280, Height: 720},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(preset.ID)
}

func ExampleBillingService_UpdateSettings() {
	ctx := context.Background()
	client := transcdr.NewClient()

	// Remove the monthly spending limit: an explicit null.
	_, err := client.Billing.UpdateSettings(ctx, &transcdr.BillingSettingsParams{
		MonthlyLimitCents: transcdr.Null[int64](),
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleConstructEvent() {
	secret := os.Getenv("TRANSCDR_WEBHOOK_SECRET")
	http.HandleFunc("/hooks/transcdr", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		event, err := transcdr.ConstructEvent(body, r.Header.Get(transcdr.SignatureHeader), secret)
		if err != nil {
			http.Error(w, "bad signature", http.StatusBadRequest)
			return
		}
		if event.Type == transcdr.EventJobCompleted {
			job, _ := event.Data.Job()
			log.Println("done:", job.ID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func ExampleIsRateLimit() {
	ctx := context.Background()
	client := transcdr.NewClient()

	_, err := client.Jobs.Get(ctx, "job_example")
	switch {
	case transcdr.IsNotFound(err):
		fmt.Println("no such job")
	case transcdr.IsRateLimit(err):
		e, _ := transcdr.AsError(err)
		wait, _ := e.RetryAfter()
		fmt.Println("retry in", wait)
	case err != nil:
		e, _ := transcdr.AsError(err)
		fmt.Println(e.Code, e.Param, e.RequestID)
	}
}
