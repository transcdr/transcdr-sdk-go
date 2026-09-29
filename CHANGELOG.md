# Changelog

## 1.0.0

Output spec v2: a declarative, explicit output specification. This release speaks v2 only.

- `OutputSpec` is declared in sections (`Kind`, `Container`, `Video`, `Audio`, `Image`, `Renditions`, `Subtitles`,
  `Trim`, `Privacy`). Nothing has a default: a spec states every field its kind, container, codec and audio handling
  need, and values that follow the source are written out (`FrameRateSource()`, `BitrateStandard`,
  `BitDepthFromColor`, `LabelBySize`, `FramesPoster()`, `GopSegment()`, `TrimEndSource()`, `SubtitlesAll()`).
- Constructors take every required field: `NewVideoOutput`, `NewAudioOutput`, `NewImageOutput`, `NewVideo`,
  `NewAudio`, `NewImage`, `NewSize`, `NewTrim`. Exclusive choices have their own constructors (`QualityLevel`,
  `ConstantRateFactor`, `ConstantBitRate`; `RenditionSizes`, `RenditionLadder`, `RenditionSourceSize`;
  `SubtitlesAll`, `SubtitlesNone`, `SubtitleLanguages`; `GopFrames`, `GopSeconds`, `GopSegment`; `FramesPoster`,
  `FramesCount`, `FramesAt`; `PrivacyPreset`, `PrivacyFields`).
- `ValidateOutput` and `ValidateOutputJSON` check a spec against the API's table of required fields and report every
  problem at once, with the API's params and messages. `Jobs.Create`, `Presets.Create` and `Presets.Replace` run it on
  a whole spec and refuse an incomplete one before sending.
- `Error.Errors` lists every problem of a refused output spec (the 422's `error.errors`).
- Jobs take either `Preset` (a slug, an id, or `"<slug>@N"` for a version) with `Overrides`, or `Output`, the whole
  spec. `Job.Preset` records the preset version and the overrides.
- Presets are versioned: `Preset.Version`, `Presets.Versions`, `Presets.GetVersion`. `PresetUpdateParams.Output` is
  overrides over the latest version.
- Automations: `Output` is `OutputOverrides`; `ResolvedOutput` is the spec they resolve to now.
- `Privacy` is a preset refined by any of its four categories (`PrivacyPreset(…).WithCaptureTime("date")`), or all four
  categories without one (`PrivacyFields`).
- `Job.Preset` carries the preset's `Slug` too.
- `Capabilities.Output` describes the spec as data: fields, when each is required, exclusive groups, containers,
  audio codecs, follow values and the v1 compatibility mode.

### Breaking

- Removed: `OutputSpecInput`, `RawOutputSpec`, `Rendition`, `Quality`, the exported fields of `ImageFrames` (use `FramesPoster`, `FramesCount`, `FramesAt`), the `Mode*`,
  `AudioMode*` and `AudioContainer*` constants, and `QualityCBR`. `FlacCompressionDefault` is `FlacCompressionBalanced`.
- A job with neither a preset nor an output is refused by the SDK (v1 read it as its default spec).
- Removed the operator console (`Client.Admin`: overview, jobs, organizations, credit, announcements and incidents).
  It is for Transcdr's own operators, not a customer API, and is no longer part of the public SDK.

### Migrating from v1

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

## 0.9.0 and earlier

See the git history and the tags.
