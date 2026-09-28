# Transcdr Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/transcdr/transcdr-sdk-go.svg)](https://pkg.go.dev/github.com/transcdr/transcdr-sdk-go)

This is the Go client for the [Transcdr](https://transcdr.com) video transcoding API. It covers every endpoint of the v1 API, the same surface as the TypeScript SDK. You can submit jobs, upload files, manage presets, connections, automations and event destinations, and verify webhook signatures.

```sh
go get github.com/transcdr/transcdr-sdk-go
```

It needs Go 1.24 or newer, and has no dependencies outside the standard library.

## Quick start

```go
import transcdr "github.com/transcdr/transcdr-sdk-go"

client := transcdr.NewClient() // reads TRANSCDR_API_KEY (and TRANSCDR_BASE_URL)

job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{
	Input:  transcdr.URLInput("https://example.com/talk.mov"),
	Preset: transcdr.String("hls-av1-abr"),
})
if err != nil {
	return err
}
job, err = client.Jobs.WaitFor(ctx, job.ID, &transcdr.WaitForOptions{
	OnProgress: func(j *transcdr.Job) { fmt.Printf("%s %.0f %%\n", j.Progress.Stage, j.Progress.Percent) },
})
fmt.Println(job.Status, *job.PlaybackURL)
```

## Configuration

```go
client := transcdr.NewClient(
	transcdr.WithAPIKey("tdk_live_…"),               // or a session token, tds_…
	transcdr.WithBaseURL("http://localhost:8080"),   // default https://api.transcdr.com
	transcdr.WithMaxRetries(3),                      // default 2
	transcdr.WithTimeout(30*time.Second),            // per attempt; default 60 s
	transcdr.WithUserAgent("my-app/1.0"),
	transcdr.WithHTTPClient(&http.Client{Transport: myTransport}),
)
```

Every method takes a `context.Context` and, last, optional per-request options:
- `transcdr.WithIdempotencyKey(key)`
- `transcdr.WithRequestTimeout(d)`
- `transcdr.WithRequestMaxRetries(n)`
- `transcdr.WithQuery(k, v)`
- `transcdr.WithRequestHeader(k, v)`

`client.Do(ctx, method, path, body, &out)` calls any endpoint directly.

## Uploads

`UploadFile` streams from any `io.Reader`, and `UploadPath` from a file. Each opens an upload session, PUTs the bytes straight to storage, completes the session and returns the ready asset:

```go
asset, err := client.Uploads.UploadPath(ctx, "talk.mov", &transcdr.UploadFileOptions{
	OnProgress: func(p transcdr.UploadProgress) { fmt.Printf("\r%.0f %%", p.Percent) },
})
job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{Input: transcdr.AssetInput(asset.ID), Preset: transcdr.String("web-av1-1080p")})
```

## Lists

A list comes back one page at a time with `List`. `All` returns an `iter.Seq2` that fetches pages as you range over it:

```go
page, err := client.Jobs.List(ctx, &transcdr.JobListParams{ListParams: transcdr.ListParams{Limit: 50}})

for job, err := range client.Jobs.All(ctx, &transcdr.JobListParams{
	Status:   transcdr.JobCompleted,
	Metadata: transcdr.Metadata{"customer": "acme"},
}) {
	if err != nil {
		return err
	}
	fmt.Println(job.ID)
}

jobs, err := transcdr.Collect(client.Jobs.All(ctx, nil), 500) // at most 500
```

## Output specifications

Output specifications are typed with `OutputSpecInput`. Constant bit rate is `Quality.Target = "cbr"`:

```go
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
```

`transcdr.RawOutputSpec([]byte(`{…}`))` sends JSON exactly as given, and `spec.Raw()` returns the JSON a spec was decoded from.

### Audio: AAC, lossless, MP3, audio-only, channels

`Audio.Mode` is `AudioModeAuto` (the default: compatible audio passes through, the rest becomes Opus),
`AudioModeOpus`, `AudioModeAAC`, `AudioModeMP3`, `AudioModeFLAC`, `AudioModeALAC` or `AudioModeDrop`.

- `AudioModeAAC` is AAC-LC, the audio that plays on the most devices: every browser, iPhone, Android phone and TV. An
  AAC source passes through. It works in a single MP4, HLS and audio-only `.m4a` output. `Bitrate` is 8k to 288k per
  main channel (the LFE of 5.1 and 7.1 does not count); the default is 64k mono, 128k stereo, 384k 5.1 and 512k 7.1.
- `AudioModeFLAC` and `AudioModeALAC` are lossless: a source already in that codec is copied, and they take no
  `Bitrate`. Both work in a single MP4, HLS and audio-only output. `Audio.BitDepth` is `AudioBitDepthSource` (the
  default: 16-bit for a 16-bit or lossy source, 24-bit for a deeper one), `AudioBitDepth16` or `AudioBitDepth24`. For
  FLAC, `Audio.FlacCompression` is `FlacCompressionFast`, `FlacCompressionDefault` or `FlacCompressionBest`: the same
  audio either way, a smaller file for more work.
- `AudioModeMP3` is constant bit rate, stereo at most, in a single MP4 or an audio-only output (not HLS), at 32k, 40k,
  48k, 56k, 64k, 80k, 96k, 112k, 128k, 160k, 192k, 224k, 256k or 320k (default 128k stereo, 64k mono).

`Mode: transcdr.ModeAudio` writes the audio alone as one file (label `audio`, width and height 0), billed per output
minute at the SD rate. `Audio.Container` picks the file: `AudioContainerAuto` (the default) follows the codec, a
`.flac` for FLAC, an `.m4a` for ALAC and an `.mp3` otherwise (auto audio is then MP3); `AudioContainerM4A` holds any
codec (auto audio in an `.m4a` is Opus); `AudioContainerFLAC` holds FLAC only and `AudioContainerMP3` MP3 only. The
file is `audio.mp3` (`audio/mpeg`), `audio.flac` (`audio/flac`) or `audio.m4a` (`audio/mp4`). `Container` applies only
to `ModeAudio`. A `single` job whose input has no video becomes audio-only by itself; with AAC or Opus audio it is an
`.m4a`.

`Audio.Channels` is `ChannelsSource` (the default), `ChannelsMono`, `ChannelsStereo`, `ChannelsSurround51` or
`ChannelsSurround71`; it downmixes and never upmixes. In HLS with surround audio,
`Audio.StereoFallback = transcdr.Bool(true)` adds a stereo rendition to the same audio group.

```go
// A podcast episode from a video recording.
job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{
	Input: transcdr.AssetInput("ast_..."),
	Output: &transcdr.OutputSpecInput{
		Mode:  transcdr.ModeAudio,
		Audio: &transcdr.Audio{Mode: transcdr.AudioModeMP3, Bitrate: transcdr.String("128k"), Channels: transcdr.ChannelsStereo},
	},
})

// AAC in an .m4a for phones and browsers.
m4a := &transcdr.OutputSpecInput{
	Mode:  transcdr.ModeAudio,
	Audio: &transcdr.Audio{Mode: transcdr.AudioModeAAC, Container: transcdr.AudioContainerM4A},
}

// A lossless 24-bit FLAC master.
master := &transcdr.OutputSpecInput{
	Mode:  transcdr.ModeAudio,
	Audio: &transcdr.Audio{Mode: transcdr.AudioModeFLAC, BitDepth: transcdr.AudioBitDepth24, FlacCompression: transcdr.FlacCompressionBest},
}

// Surround AAC in HLS with a stereo rendition beside it.
surround := &transcdr.OutputSpecInput{
	Mode:  transcdr.ModeHLS,
	Codec: "h264",
	Audio: &transcdr.Audio{Mode: transcdr.AudioModeAAC, Channels: transcdr.ChannelsSurround51, StereoFallback: transcdr.Bool(true)},
}
```

Audio system presets (`CategoryAudio`): `audio-mp3-podcast` and `audio-mp3-speech` (MP3 at 128k stereo and 64k mono),
`audio-aac-m4a` (AAC in an `.m4a`) and `audio-alac-m4a` (Apple Lossless in an `.m4a`). In `CategoryArchive`,
`audio-flac` is a native `.flac` at best compression and `archive-av1-flac` is visually lossless AV1 with FLAC audio in
one MP4. The reach presets (`mp4-h264-compat-1080p`, `mp4-h265-1080p`, `hls-h264-abr`, `hls-h264-cbr`,
`social-vertical-1080x1920`, `hls-h264-surround` and `mp4-h264-surround-1080p`, now in `CategoryTV`) use AAC audio.

## Explicit nulls

Where the API clears a value with `null`, the field is a `transcdr.Nullable[T]`:
- leave it out (the zero value)
- send `transcdr.Null[T]()`
- or send `transcdr.Value(v)`

```go
// Remove the monthly spending limit.
client.Billing.UpdateSettings(ctx, &transcdr.BillingSettingsParams{MonthlyLimitCents: transcdr.Null[int64]()})

// Clear a connection's folder; the rest of its config is kept (configs merge).
client.Connections.Update(ctx, id, &transcdr.ConnectionUpdateParams{
	Config: &transcdr.ConnectionConfig{Root: transcdr.Null[string]()},
})

// Stop delivering an automation's outputs to a connection.
client.Automations.Update(ctx, id, &transcdr.AutomationParams{
	Destination: transcdr.Null[transcdr.JobDestination](),
})
```

On update, `null` clears:
- an automation's `Destination`, `Preset`, `Output`, `Metadata`, `WebhookURL` and `TriggerConnectionID`
- an event destination's `Description`, `AWS.Endpoint` and `AWS.MessageGroupID`
- a connection's `Config` fields
- a preset's `Description` and `Metadata`; for `Category`, `Compatibility` and `CompatibilityNotes`, `null` means derive it from the output again
- the organization's `BillingEmail`

### Replacing a preset

`Presets.Update` (PATCH) merges `Output` into the stored specification, so a field you leave out keeps its value. `Presets.Replace` (PUT) sets the whole preset: `Output` is merged over the defaults instead, a description or metadata left out is emptied, and the slug is kept unless you set it.

```go
client.Presets.Replace(ctx, id, &transcdr.PresetReplaceParams{
	Name:   "Web H.264",
	Output: transcdr.RawOutputSpec([]byte(`{"mode":"hls","codec":"h264"}`)),
})
```

### Where a preset plays

Every preset has a `Category`, the group it is shown in: `CategoryWeb`, `CategoryMobile`, `CategoryStreaming`, `CategoryTV`, `CategorySocial`, `CategoryAudio` or `CategoryArchive`. It also has `Compatibility`, the platforms its output plays on: `PlatformWeb`, `PlatformIOS`, `PlatformAndroid`, `PlatformSmartTV`, `PlatformLegacy` and `PlatformEditing`. `CompatibilityNotes` gives each listed platform its minimum versions and conditions, such as sound that needs AAC source audio. The API derives all three from the output specification, and more values may be added over time.

```go
// Presets that play on both iOS and Android, in the web or mobile groups.
page, err := client.Presets.List(ctx, &transcdr.PresetListParams{
	Category:       []transcdr.PresetCategory{transcdr.CategoryWeb, transcdr.CategoryMobile},
	CompatibleWith: []transcdr.Platform{transcdr.PlatformIOS, transcdr.PlatformAndroid},
})
```

`Category` matches any of the values you pass. `CompatibleWith` requires every one of them. `List` and `All` still accept a plain `*transcdr.ListParams`.

Your own presets can set their own values. Leave a field out and the API derives it. On update, `transcdr.Null` derives it again:

```go
client.Presets.Update(ctx, id, &transcdr.PresetUpdateParams{
	Category:           transcdr.Value(transcdr.CategoryStreaming),
	Compatibility:      transcdr.Value([]transcdr.Platform{transcdr.PlatformSmartTV}),
	CompatibilityNotes: transcdr.Null[map[transcdr.Platform]string](),
})
```

## Write-only secrets

Connections and event destinations never return their secrets. `Secrets` has an entry for each one that is set, with a fingerprint: an HMAC keyed on the server and bound to the object and the field. It cannot be computed or checked on the client, but it changes whenever the secret does, so comparing it with an earlier read shows a secret replaced elsewhere:

```go
if transcdr.SecretChanged(saved.Secrets, conn.Secrets, "secret_access_key") {
	// set, cleared or rotated since `saved` was read
}
```

## Errors

API errors are `*transcdr.Error` values carrying:
- the HTTP `Status` and error `Type`
- `Code`, e.g. `validation_failed` or `insufficient_credit`
- `Param` and per-field `Details`
- the `RequestID` to quote to support

`RetryAfter()` reads the `Retry-After` header on 429s. If no response arrives, the error is a `*transcdr.ConnectionError` instead.

```go
_, err := client.Jobs.Create(ctx, params)
if e, ok := transcdr.AsError(err); ok {
	log.Printf("%s %s (%s): %v [request %s]", e.Type, e.Code, e.Param, e.Details, e.RequestID)
}
switch {
case transcdr.IsQuota(err):      // 402: add credit
case transcdr.IsRateLimit(err):  // 429
case transcdr.IsNotFound(err):   // 404
case transcdr.IsConnection(err): // no response
}
```

### Retries

A request is retried with jittered exponential backoff after a 429, a 5xx or a network error, honouring `Retry-After`. Only requests that are safe to repeat are retried:
- GET, PUT and DELETE requests
- POSTs carrying an `Idempotency-Key`

Every create sends an `Idempotency-Key` automatically:
- jobs, probes, uploads and assets
- presets, event destinations, connections and automations
- API keys, members and organizations

The API remembers the key for 24 hours per organization and replays the first successful response (with `Idempotent-Replayed: true`), so a retried create never makes a duplicate. To make a create safe to repeat across processes or restarts, pass your own key with `transcdr.WithIdempotencyKey`. Reusing a key for a different request is a 409 with code `idempotency_key_reused`.

## Webhooks

Every delivery is signed. Verify the raw body before trusting it:

```go
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
})
```

SNS and SQS destinations carry the same signature in the `transcdr-signature` message attribute. `VerifySNSSQSSignature` accepts the attribute map in any shape AWS hands over: `ReceiveMessage` and the AWS SDKs, Lambda events, SNS JSON, or a plain string:

```go
ok := transcdr.VerifySNSSQSSignature([]byte(record.Body), record.MessageAttributes, secret)
```

Signatures are `t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>`, the same scheme as the TypeScript SDK, and are checked in constant time with a 5-minute tolerance.

## What's covered

| Area | Services |
|---|---|
| Account | `Auth` (register, login, switch, logout, password, me), `Organization` (+ `Members`), `Organizations`, `APIKeys` |
| Media | `Uploads` (with `UploadFile`/`UploadPath`), `Assets`, `Jobs` (create, list, get, cancel, retry, delete, events, outputs, output and file URLs, deliveries, deliver, `WaitFor`), `Probe`, `Presets` |
| Events | `Webhooks` (event destinations: HTTPS, SNS, SQS, connections; check, test, rotate secret, deliveries, redeliver), `Events` |
| Billing | `Usage` (with the `Inputs` report), `Billing` (get, checkout, portal, settings, transactions, change plan, `Invoices`), `Plans` |
| Service | `Capabilities`, `Status`, `Stats`, `Changelog`, `Announcements`, `OpenAPI` |
| Integrations | `Connections` (check, test, browse, enable, disable), `Automations` (run, trigger, rotate hook token, items, `PushHook`), `Deliveries` |
| Operators | `Admin` (overview, jobs, organizations, credit, `Announcements`, `Incidents`) |

## Development

```sh
go test ./...
# an API to test against, e.g. a local development server:
TRANSCDR_INTEGRATION=1 TRANSCDR_BASE_URL=http://localhost:8080 TRANSCDR_API_KEY=tdk_test_… go test -run Integration -v
```

The decoding tests in `testdata/fixtures` hold recorded API responses, scrubbed of personal data. Every fixture must decode strictly into its type.

## License

MIT
