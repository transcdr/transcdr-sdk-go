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

	// Constant bit rate HLS: each size at its own rate, else video.cbr's.
	// Every field is stated; an incomplete spec is refused before it is sent.
	spec := transcdr.NewVideoOutput(
		transcdr.ContainerHLS(6),
		transcdr.NewVideo(transcdr.CodecH264, transcdr.ConstantBitRate("3M", 1000), transcdr.BitDepth8,
			transcdr.ColorSDR, transcdr.FrameRateSource(), transcdr.GopSegment(), nil),
		transcdr.Audio{Handling: transcdr.HandlingEncode, Codec: transcdr.AudioCodecAAC, Bitrate: transcdr.BitrateStandard,
			Channels: transcdr.ChannelsSource, HeAac: transcdr.HeAacAuto, StereoFallback: transcdr.Bool(false)},
		transcdr.RenditionSizes(
			transcdr.NewSize(transcdr.LabelBySize, 1920, 1080, transcdr.FitContain, transcdr.OrientationAuto, false).WithBitrate("6M"),
			transcdr.NewSize(transcdr.LabelBySize, 1280, 720, transcdr.FitContain, transcdr.OrientationAuto, false),
		),
		transcdr.SubtitlesAll(),
		transcdr.NewTrim(0, transcdr.TrimEndSource()),
		transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
	)
	preset, err := client.Presets.Create(ctx, &transcdr.PresetCreateParams{Name: "Broadcast CBR", Output: &spec})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(preset.ID, preset.Version)
}

func ExampleJobsService_Create_overrides() {
	ctx := context.Background()
	client := transcdr.NewClient()

	// A preset version, with one field changed.
	_, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{
		Input:     transcdr.URLInput("https://example.com/talk.mov"),
		Preset:    transcdr.String("social-vertical-1080x1920@1"),
		Overrides: transcdr.OutputOverrides{"video": map[string]any{"frame_rate": map[string]any{"max": 24}}},
	})
	if e, ok := transcdr.AsError(err); ok && e.Code == "validation_failed" {
		for _, f := range e.Errors {
			fmt.Println(f.Param, f.Message)
		}
	}
}

func ExampleValidateOutput() {
	spec := transcdr.NewAudioOutput(
		transcdr.ContainerAudio(transcdr.FormatMP3),
		transcdr.NewAudio(transcdr.HandlingEncode, transcdr.AudioCodecMP3, transcdr.ChannelsMono, transcdr.HeAacAuto),
		transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
	)
	for _, e := range transcdr.ValidateOutput(spec) {
		fmt.Println(e.Param)
	}
	// Output: output.audio.bitrate
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
