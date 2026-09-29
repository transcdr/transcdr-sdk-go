# Transcdr Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/transcdr/transcdr-sdk-go.svg)](https://pkg.go.dev/github.com/transcdr/transcdr-sdk-go)

This is the Go client for the [Transcdr](https://transcdr.com) video transcoding API. It covers every endpoint of the v1 API, the same surface as the TypeScript SDK, and speaks output spec v2 (see [Migrating from v1](#migrating-from-v1)). You can submit jobs, upload files, manage presets, connections, automations and event destinations, and verify webhook signatures.

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

An `OutputSpec` says what a job produces, in sections: `Kind` (`KindVideo`, `KindAudio` or `KindImage`), `Container`,
`Video`, `Audio`, `Image`, `Renditions`, `Subtitles`, `Trim` and `Privacy`. This is output spec v2.

**Nothing has a default.** A spec states every field its kind, container, codec and audio handling need. A value that
follows the source is written out: `FrameRateSource()`, `ChannelsSource`, `BitrateStandard`, `BitDepthFromColor`,
`LabelBySize`, `FramesPoster()`, `GopSegment()`, `TrimEndSource()`, `SubtitlesAll()`. The SDK never fills a field in.

Build a spec with the constructor for its kind, which takes every section that kind needs:

| Constructor | Sections |
|---|---|
| `NewVideoOutput` | container, video, audio, renditions, subtitles, trim, privacy |
| `NewAudioOutput` | container, audio, privacy |
| `NewImageOutput` | image, renditions, privacy |

Exclusive choices are built by constructors, so exactly one is set:

| Section | Choices |
|---|---|
| video rate | `QualityLevel(QualityHigh)` or `QualityLevel(QualityVMAF(93))`, `ConstantRateFactor(23)`, `ConstantBitRate("5M", 1000)` |
| renditions | `RenditionSizes(sizes...)`, `RenditionLadder(maxShortSide, fit, upscale)`, `RenditionSourceSize(label, fit, upscale)` |
| subtitles | `SubtitlesAll()`, `SubtitlesNone()`, `SubtitleLanguages("eng", "deu")` |
| gop | `GopSeconds(2)`, `GopFrames(48)`, `GopSegment()` (HLS: one keyframe per segment) |
| frame rate | `FrameRateSource()`, `MaxFrameRate(30)` |
| image frames | `FramesPoster()`, `FramesCount(12)`, `FramesAt(1.5, 10)` |
| trim end | `TrimEndSource()`, `TrimEndAt(7.5)` |
| privacy | `PrivacyPreset(PrivacyStripAll)`, or all four categories with `PrivacyFields(location, captureTime, device, descriptive)` |

### Examples

An adaptive HLS ladder of H.264 at a constant bit rate:

```go
spec := transcdr.NewVideoOutput(
	transcdr.ContainerHLS(6),
	transcdr.NewVideo(transcdr.CodecH264, transcdr.ConstantBitRate(transcdr.BitrateStandard, 1000), transcdr.BitDepth8,
		transcdr.ColorSDR, transcdr.FrameRateSource(), transcdr.GopSegment(), nil),
	transcdr.Audio{Handling: transcdr.HandlingEncode, Codec: transcdr.AudioCodecAAC, Bitrate: transcdr.BitrateStandard,
		Channels: transcdr.ChannelsSource, HeAac: transcdr.HeAacAuto, StereoFallback: transcdr.Bool(false)},
	transcdr.RenditionSizes(
		transcdr.NewSize(transcdr.LabelBySize, 1920, 1080, transcdr.FitContain, transcdr.OrientationAuto, false).WithBitrate("5M"),
		transcdr.NewSize(transcdr.LabelBySize, 1280, 720, transcdr.FitContain, transcdr.OrientationAuto, false).WithBitrate("3M"),
	),
	transcdr.SubtitlesAll(),
	transcdr.NewTrim(0, transcdr.TrimEndSource()),
	transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
)
job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{Input: transcdr.AssetInput("ast_..."), Output: &spec})

// An automatic ladder instead of the two sizes:
ladder := transcdr.RenditionLadder(1080, transcdr.FitContain, false)
spec.Renditions = &ladder
```

A single portrait MP4 for social apps, capped at 30 fps:

```go
spec := transcdr.NewVideoOutput(
	transcdr.ContainerMP4(),
	transcdr.NewVideo(transcdr.CodecH264, transcdr.QualityLevel(transcdr.QualityHigh), transcdr.BitDepthFromColor,
		transcdr.ColorSDR, transcdr.MaxFrameRate(30), transcdr.GopSeconds(2), nil),
	transcdr.Audio{Handling: transcdr.HandlingEncode, Codec: transcdr.AudioCodecAAC, Bitrate: transcdr.BitrateStandard,
		Channels: transcdr.ChannelsSource, HeAac: transcdr.HeAacAuto},
	transcdr.RenditionSizes(transcdr.NewSize(transcdr.LabelBySize, 1080, 1920, transcdr.FitCover, transcdr.OrientationFixed, false)),
	transcdr.SubtitlesAll(),
	transcdr.NewTrim(0, transcdr.TrimEndSource()),
	transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
)
```

An audio-only MP3, and a lossless FLAC master:

```go
podcast := transcdr.NewAudioOutput(
	transcdr.ContainerAudio(transcdr.FormatMP3),
	transcdr.Audio{Handling: transcdr.HandlingEncode, Codec: transcdr.AudioCodecMP3, Bitrate: "64k",
		Channels: transcdr.ChannelsMono, HeAac: transcdr.HeAacAuto},
	transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
)

master := transcdr.NewAudioOutput(
	transcdr.ContainerAudio(transcdr.FormatFLAC),
	transcdr.Audio{Handling: transcdr.HandlingEncode, Codec: transcdr.AudioCodecFLAC, Channels: transcdr.ChannelsSource,
		HeAac: transcdr.HeAacAuto, BitDepth: transcdr.AudioBitDepth24, FlacCompression: transcdr.FlacCompressionBest},
	transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
)
```

Twelve evenly spaced JPEG stills of a video:

```go
stills := transcdr.NewImageOutput(
	transcdr.Image{Formats: []transcdr.ImageFormat{transcdr.ImageFormatJPEG}, Quality: map[transcdr.ImageFormat]int{transcdr.ImageFormatJPEG: 80},
		ColorProfile: transcdr.ColorProfileSRGB, Frames: transcdr.FramesCount(12)},
	transcdr.RenditionSizes(transcdr.NewSize("sheet", 320, 320, transcdr.FitContain, transcdr.OrientationAuto, false)),
	transcdr.PrivacyPreset(transcdr.PrivacyStripAll),
)
```

### Which fields a spec needs

| Field | Required when |
|---|---|
| `Kind`, `Privacy` | always (`Privacy`: a preset, or all of location, capture time, device and descriptive) |
| `Container.Format` | kind video (`FormatMP4`, `FormatHLS`) or audio (`FormatMP3`, `FormatFLAC`, `FormatM4A`) |
| `Container.SegmentSeconds` | format HLS (1–20) |
| `Video.Codec`, `BitDepth`, `Color`, `FrameRate`, `Gop`, `Filters`, and one rate | kind video |
| `Audio.Handling` | kind video or audio (`HandlingDrop` is for video only) |
| `Audio.Codec`, `Channels`, `HeAac` | handling auto or encode |
| `Audio.Bitrate` | codec opus, mp3 or aac |
| `Audio.StereoFallback` | HLS, with an audio track |
| `Audio.BitDepth` | codec flac or alac |
| `Audio.FlacCompression` | codec flac |
| `Image.Formats`, `ColorProfile`, `Frames` | kind image |
| `Image.Lossless` | WebP among the formats |
| `Image.Quality` | a lossy format is made: one entry per lossy format made (`{avif: 60, jpeg: 82}`) |
| one of sizes, ladder, source size | kind video or image (the ladder is for video only) |
| `Subtitles`, `Trim` | kind video |

A field given where it does not apply is refused too. `transcdr.ValidateOutput(spec)` checks a spec against this
table, which is the same one the API lists in `Capabilities.Output`, and reports every problem at once with the API's
params and messages. `Jobs.Create`, `Presets.Create` and `Presets.Replace` run it on a whole spec before sending: an
incomplete spec returns a `*transcdr.Error` with code `validation_failed`, `Status` 0 and every problem in `Errors`, and
nothing is sent. Values checked against each other (HDR colour with 8-bit, MP3 in HLS) and plan limits are the API's to
check.

```go
for _, e := range transcdr.ValidateOutput(spec) {
	fmt.Println(e.Param, e.Message) // output.audio.bitrate output.audio.bitrate is required when …
}
```

Values that follow the source:

| Value | Resolves to |
|---|---|
| `FrameRateSource()` | the source's frame rate, not capped |
| `BitDepthFromColor` | 8-bit for SDR, 10-bit for HDR10 and HLG, the source's for passthrough |
| `ConstantBitRate(BitrateStandard, …)` | a rate per size by codec, short side and frame rate: H.264 at 30 fps about 5M at 1080p, 3M at 720p, 1.2M at 480p, 0.8M at 360p; H.265 about 0.65× that, AV1 about 0.5×; more above 30 fps |
| `GopSegment()` | HLS: a keyframe at the start of each segment and none inside it |
| audio `BitrateStandard` | AAC 64k mono, 128k stereo, 384k 5.1, 512k 7.1; Opus 96k stereo, 320k 5.1, 416k 7.1; MP3 64k mono, 128k stereo |
| `ChannelsSource` | the source's layout (MP3 folds a wider one to stereo) |
| `AudioBitDepthSource` | 16-bit for a 16-bit or lossy source, 24-bit for a deeper one |
| `HeAacAuto` | an HE-AAC source passes through where only a codec change is asked; its AAC-LC core is decoded where the job needs PCM |
| `LabelBySize` | `<short side>p` of the size it comes out at; for images `<width>x<height>` |
| `TrimEndSource()` | the end of the source |
| `FramesPoster()` | an image input as it is; a video's frame 10% of the way in |
| `SubtitlesAll()` | every subtitle track |

### Sizes are maximums

A size's `Width` x `Height` is the largest the output may be. The picture keeps its shape inside the box with
`FitContain`; `FitCover` fills the box and centre-crops; `FitPad` adds black bars to exactly the box; `FitStretch`
distorts to it. `OrientationAuto` turns the box to the picture's orientation (1920x1080 on a portrait video is
1080x1920), `OrientationFixed` uses it as written. Nothing is enlarged past the source unless `Upscale` is true. Each
output reports the size it came out at.

### Audio

`HandlingAuto` keeps compatible audio as it is and makes the rest `Codec`, which is `AudioCodecOpus`
(`AudioCodecMP3` in an `.mp3`); `HandlingEncode` makes `Codec`, copying a source already in it when nothing else
changes; `HandlingDrop` makes no audio track (`transcdr.AudioDrop()`).

- `AudioCodecAAC` is AAC-LC, which plays on the most devices. It takes 8k to 288k per main channel.
- `AudioCodecMP3` is constant bit rate at 32k, 40k, 48k, 56k, 64k, 80k, 96k, 112k, 128k, 160k, 192k, 224k, 256k or
  320k, stereo at most; for an MP4 or an `.mp3`, not HLS.
- `AudioCodecFLAC` and `AudioCodecALAC` are lossless and take no bitrate; `BitDepth` is `AudioBitDepthSource`, `16` or
  `24`, and FLAC's `FlacCompression` (`fast`, `balanced`, `best`) trades time for size.
- `Channels` downmixes and never upmixes. `StereoFallback` adds, in HLS beside surround audio, a stereo downmix in the
  same audio group.
- HE-AAC is decoded only as its AAC-LC core. `HeAacPassthrough` never decodes it, failing a job that would need it;
  `HeAacCore` decodes its core whenever another codec or a change is asked.

`KindAudio` writes the audio alone as one file, `audio.mp3`, `audio.flac` or `audio.m4a` (an `.mp3` holds MP3 only, a
`.flac` FLAC only, an `.m4a` any codec), billed per output minute at the SD rate.

### Image jobs

`KindImage` makes still images, of an image input (JPEG, PNG, WebP, AVIF, GIF, TIFF, BMP, HEIC) or taken from a video.
Every size is made in every format of `Image.Formats`: one to four of `ImageFormatAVIF`, `ImageFormatWebP`,
`ImageFormatJPEG` and `ImageFormatPNG`. Image sizes are 16 to 8192 on a side, odd sizes allowed.

- `Image.Quality` names each lossy format made, 1 to 100. `Image.Lossless` is required with WebP; PNG always is lossless.
- Outputs are upright, sRGB unless `ColorProfile` is `ColorProfileKeep`, and carry no identifying metadata unless
  `Privacy` keeps a category.
- Each `JobOutput` carries its `Format`, its `Rendition`, and for a video's stills its `Frame` (from 1) and `AtSeconds`.
- Images are billed per output image by the pixels it came out at: `JobBilling.BillableImages` counts them and
  `JobBilling.Tier` is `ImageTierUpTo1MP`, `ImageTierUpTo4MP` or `ImageTierOver4MP`.

Image system presets (`CategoryImage`): `web-avif` and `web-webp`, `thumbnail-jpeg`, `png-lossless`, `video-poster` and
`contact-sheet`.

## Presets, versions and overrides

A preset is a complete spec, and presets are versioned: editing a preset's output adds a version, and a version never
changes. `Preset.Version` is the latest. A job names one by system slug (`hls-av1-abr`), your preset's slug or `pre_…`
id for its latest version, or `"<slug>@N"` for version N.

```go
versions, err := client.Presets.Versions(ctx, "web-avif") // every version, oldest first
first, err := client.Presets.GetVersion(ctx, "web-avif", 1) // one version
```

With a preset, `Overrides` give only what to change, as a JSON merge document:

```go
job, err := client.Jobs.Create(ctx, &transcdr.JobCreateParams{
	Input:     transcdr.URLInput("https://example.com/talk.mov"),
	Preset:    transcdr.String("social-vertical-1080x1920@1"),
	Overrides: transcdr.OutputOverrides{"video": map[string]any{"frame_rate": map[string]any{"max": 24}}},
})
```

| In `Overrides` | Effect |
|---|---|
| an object | merged key by key |
| a scalar or an array (`sizes`, `formats`, `filters`, …) | replaces |
| one choice of a group (`quality`/`crf`/`cbr`, `sizes`/`ladder`/`source_size`, `tracks`/`languages`) | replaces the others |
| `privacy.preset` | replaces the preset's privacy |
| `nil` (JSON null) | removes the field |
| `kind` | cannot change |

The result must be complete: a field left dangling (say `segment_seconds` after switching to `mp4`) is refused by name,
so set it to `nil` too. A job gives either `Preset` (with `Overrides`, if any) or `Output`, the whole spec; the SDK
refuses neither and both. Every job records `Preset` (`ID`, `Version`, `Overrides`), nil when given whole, and its
`Output` is always the resolved, complete spec, which is what runs.

`Presets.Update` (PATCH) merges `Output` overrides over the latest version, so a field you leave out keeps its value.
`Presets.Create` and `Presets.Replace` (PUT) take the whole spec. An automation's `Output` is overrides over its
preset, and `ResolvedOutput` the spec they resolve to now.

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
- in `Overrides` and `OutputOverrides`, a field (as `nil`)
- a preset's `Description` and `Metadata`; for `Category`, `Compatibility` and `CompatibilityNotes`, `null` means derive it from the output again
- the organization's `BillingEmail`

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
- `Errors`: every problem with a refused output spec, missing fields first (`Param` and `Message` are the first)
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

## Migrating from v1

1.0.0 speaks output spec v2 only. The v1 shape (`Mode`, `Codec`, `Quality.Target`, `Renditions` as a list, `Audio.Mode`,
top-level `Fit` and `Upscale`, `Ladder`, …) and `OutputSpecInput`, `RawOutputSpec`, `Rendition` and `Quality` are
gone. A v2 spec states every field; the v1 defaults are in the right-hand column, so writing them out gives exactly
what a v1 request got.

| v1 | v2 | v1 default, written out in v2 |
|---|---|---|
| `mode: single` | `kind: video`, `container.format: mp4` | `single` |
| `mode: hls` | `kind: video`, `container.format: hls` | |
| `segment_seconds` | `container.segment_seconds` | `4` |
| `mode: audio` | `kind: audio` | |
| `audio.container` | `container.format` (`mp4` read as `m4a`) | `auto` → `flac` for flac, `m4a` for alac, else `mp3` |
| `mode: image` | `kind: image` | |
| `codec` | `video.codec` | `av1` |
| `quality.target` (a level) | `video.quality` | none set → `quality: "standard"` |
| `quality.crf` | `video.crf` (a level `target` is dropped: crf won) | |
| `quality.target: cbr` | `video.cbr` | |
| `quality.bitrate` | `video.cbr.bitrate` | `"standard"` |
| `quality.buffer_ms` | `video.cbr.buffer_ms` | `1000` |
| `bit_depth` | `video.bit_depth` (`auto` → `from_color`) | `from_color` |
| `color` | `video.color` | `sdr` |
| `max_fps` | `video.frame_rate.max` | `"source"` |
| `gop` | `video.gop.frames` | mp4: `{ seconds: 2 }`; hls: `"segment"` |
| `filters: "a,b"` | `video.filters: ["a", "b"]` | `[]` |
| `renditions[]` | `renditions.sizes[]` | none and no ladder → `source_size`, with the top-level `fit` and `upscale` |
| `renditions[].label` | `sizes[].label` | `by_size` |
| `renditions[].fit` / `upscale` | `sizes[].fit` / `upscale` | the top-level `fit` / `upscale`, which default to `contain` / `false` |
| `renditions[].orientation` | `sizes[].orientation` | `auto` |
| `renditions[].bitrate` | `sizes[].video.cbr.bitrate` | |
| `fit`, `upscale` (top level) | written onto every size, the ladder or the source size; dropped for audio | `contain`, `false` |
| `ladder` | `renditions.ladder` (dropped when `renditions` is non-empty, as v1 ignored it) | `max_short_side` → `1080` |
| `audio.mode: auto` | `handling: auto`, `codec: opus` (`mp3` in an mp3 container) | |
| `audio.mode: opus` \| `mp3` \| `aac` \| `flac` \| `alac` | `handling: encode`, `codec` | |
| `audio.mode: drop` | `handling: drop` | |
| `audio.bitrate` | `audio.bitrate` | `"standard"` (lossy) |
| `audio.channels` | `audio.channels` | `source` |
| `audio.he_aac` | `audio.he_aac` | `auto` |
| `audio.stereo_fallback` | `audio.stereo_fallback` | `false` (hls) |
| `audio.bit_depth` | `audio.bit_depth` | `source` (flac/alac) |
| `audio.flac_compression` | `audio.flac_compression` (`default` → `balanced`) | `balanced` (flac) |
| `subtitles: all\|none` | `subtitles.tracks` | `all` |
| `subtitles: "eng,deu"` | `subtitles.languages` | |
| `trim` | `trim` | `{ start: 0, end: "source" }`; `end` unset → `"source"` |
| `image.formats` | `image.formats` | `["avif"]` |
| `image.quality: 70` | `image.quality: { <each lossy format>: 70 }` | avif 60, webp 80, jpeg 82 |
| `image.lossless` | `image.lossless` | `false` (webp) |
| `image.keep_color_profile` | `image.color_profile: keep \| srgb` | `srgb` |
| `image.frames` | `image.frames` | `"poster"` |
| `privacy` | `privacy`, all four fields resolved | `{ preset: "strip_all" }` |

In Go:
- `JobCreateParams.Output` is a whole `*OutputSpec`; over a preset, use `Overrides` (`OutputOverrides`, a JSON merge
  document). A job needs one of `Preset` and `Output`; one with neither, which v1 read as its default spec, is refused.
- `PresetCreateParams.Output` and `PresetReplaceParams.Output` are a whole `*OutputSpec`; `PresetUpdateParams.Output`
  and `AutomationParams.Output` are `OutputOverrides`.
- `Job.Preset` is new (`PresetProvenance`), as are `Preset.Version`, `Presets.Versions`, `Presets.GetVersion`,
  `Automation.ResolvedOutput`, `Capabilities.Output`, `Error.Errors` and `ValidateOutput`.
- `FlacCompressionDefault` is now `FlacCompressionBalanced`; `AudioContainer*` are `Format*` container formats;
  `Audio.Mode` is `Audio.Handling` plus `Audio.Codec`; `Image.KeepColorProfile` is `Image.ColorProfile`.
- Webhook payloads the API stored before v2 keep their v1 `output`; `Event.Data.Job()` decodes them only in v2, so read
  such an event's raw `Data.Object`.

**Older SDK versions keep working.** The API still accepts v1 requests with their defaults, and 0.x releases of this
SDK, whose fields are all optional and would read a v2 response as an empty spec, can ask for responses in the v1
shape with the `Transcdr-Output-Spec: v1` header (or `?output_spec=v1` on a GET), with
`transcdr.WithHeader("Transcdr-Output-Spec", "v1")`. That compatibility mode is deprecated from
the start: its responses carry `Deprecation: true` and a `Sunset` date, and it is removed after 31 March 2027. Move to
1.0.0 before then.

## Development

```sh
go test ./...
# an API to test against, e.g. a local development server:
TRANSCDR_INTEGRATION=1 TRANSCDR_BASE_URL=http://localhost:8080 TRANSCDR_API_KEY=tdk_test_… go test -run Integration -v
```

The decoding tests in `testdata/fixtures` hold recorded API responses, scrubbed of personal data. Every fixture must decode strictly into its type.

## License

MIT
