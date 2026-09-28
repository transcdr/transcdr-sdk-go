package transcdr

import (
	"encoding/json"
	"time"
)

// Metadata is string-to-string metadata: up to 20 keys (≤ 40 characters)
// with values of at most 500 characters.
type Metadata map[string]string

// Rendition is one rung of the output ladder.
//
// Width x Height is the largest the rendition may be: the video keeps its
// shape inside that box (see OutputSpec.Fit) and is not enlarged past its own
// size unless Upscale is on. Each output reports the size it came out at.
type Rendition struct {
	// Width is the maximum width; even, 64–7680.
	Width int `json:"width"`
	// Height is the maximum height; even, 64–4320.
	Height int `json:"height"`
	// Bitrate is this rendition's constant rate, such as "3M" or "800k"
	// (100k–200M), with quality target "cbr" only.
	Bitrate *string `json:"bitrate,omitempty"`
	// Label is 1–32 of [A-Za-z0-9_-]; the API's default is "<short side>p"
	// of the size the rendition comes out at.
	Label *string `json:"label,omitempty"`
	// Fit is this rendition's own fit (a Fit* constant), over OutputSpec.Fit.
	Fit string `json:"fit,omitempty"`
	// Orientation is OrientationAuto or OrientationFixed: fixed keeps this
	// rendition's box as written, e.g. a 9:16 cover rendition that crops a
	// landscape video.
	Orientation string `json:"orientation,omitempty"`
	// Upscale is this rendition's own upscale, over OutputSpec.Upscale.
	Upscale *bool `json:"upscale,omitempty"`
}

// How the video meets a rendition's box (OutputSpec.Fit, Rendition.Fit).
const (
	// FitContain keeps the video's shape inside the box. The default.
	FitContain = "contain"
	// FitCover fills the box, keeping the shape, and centre-crops the rest.
	FitCover = "cover"
	// FitPad keeps the shape and adds black bars to exactly the box.
	FitPad = "pad"
	// FitStretch distorts the picture to exactly the box.
	FitStretch = "stretch"
)

// Whether a rendition's box turns to the video (Rendition.Orientation).
const (
	// OrientationAuto turns the box to the video's orientation: 1920x1080 on
	// a portrait video is 1080x1920. The default.
	OrientationAuto = "auto"
	// OrientationFixed uses the box as written.
	OrientationFixed = "fixed"
)

// Ladder asks for an automatic ABR ladder.
type Ladder struct {
	// MaxShortSide caps the tallest rung's short side, e.g. 1080.
	MaxShortSide *int `json:"max_short_side,omitempty"`
}

// Quality targets.
const (
	QualityVisuallyLossless = "visually_lossless"
	QualityHigh             = "high"
	QualityStandard         = "standard"
	QualityLow              = "low"
	// QualityCBR codes every rendition at a constant bit rate instead of to a
	// quality level: each at its own Bitrate, else Quality.Bitrate, else a
	// default for its resolution and codec.
	QualityCBR = "cbr"
)

// QualityVMAF is the target "vmaf=N" (1–100).
func QualityVMAF(score int) string { return "vmaf=" + itoa(score) }

// Quality is how hard the encode tries, or the constant rate.
type Quality struct {
	// Target is visually_lossless, high, standard, low, vmaf=N or cbr.
	Target *string `json:"target,omitempty"`
	// CRF is 0–63 and wins over Target; not with "cbr".
	CRF *int `json:"crf,omitempty"`
	// Bitrate is "cbr" only: the rate for renditions without their own, e.g. "5M".
	Bitrate *string `json:"bitrate,omitempty"`
	// BufferMs is "cbr" only: the rate buffer, 100–10000 ms (default 1000).
	BufferMs *int `json:"buffer_ms,omitempty"`
}

// Output modes (OutputSpec.Mode, OutputSpecInput.Mode).
const (
	// ModeSingle is one MP4 per rendition.
	ModeSingle = "single"
	// ModeHLS is an adaptive HLS ladder.
	ModeHLS = "hls"
	// ModeAudio is the audio alone, as one file (label "audio", width and
	// height 0): an .mp3, .flac or .m4a, as Audio.Container picks.
	ModeAudio = "audio"
)

// Audio modes (Audio.Mode).
const (
	// AudioModeAuto passes compatible audio through and transcodes the rest:
	// to Opus, or to MP3 in an audio-only .mp3.
	AudioModeAuto = "auto"
	AudioModeOpus = "opus"
	// AudioModeMP3 is constant bit rate MP3, stereo at most, in a single MP4
	// or audio-only output (not HLS).
	AudioModeMP3 = "mp3"
	// AudioModeAAC is AAC-LC, the audio that plays on the most devices; an
	// AAC source passes through.
	AudioModeAAC = "aac"
	// AudioModeFLAC is lossless FLAC (a FLAC source is copied). No bitrate.
	AudioModeFLAC = "flac"
	// AudioModeALAC is lossless ALAC, Apple Lossless (an ALAC source is
	// copied). No bitrate.
	AudioModeALAC = "alac"
	AudioModeDrop = "drop"
)

// AudioBitDepth is the sample depth of FLAC and ALAC output. Values added
// later decode as they are.
type AudioBitDepth string

// Lossless sample depths.
const (
	// AudioBitDepthSource (the default) is 16-bit for a 16-bit or lossy
	// source, 24-bit for a deeper one.
	AudioBitDepthSource AudioBitDepth = "source"
	AudioBitDepth16     AudioBitDepth = "16"
	AudioBitDepth24     AudioBitDepth = "24"
)

// FlacCompression is FLAC's compression effort: the same audio either way,
// a smaller file for more work. Values added later decode as they are.
type FlacCompression string

// FLAC compression efforts.
const (
	FlacCompressionFast FlacCompression = "fast"
	// FlacCompressionDefault is the default.
	FlacCompressionDefault FlacCompression = "default"
	FlacCompressionBest    FlacCompression = "best"
)

// AudioContainer is the file audio-only output is. Values added later
// decode as they are.
type AudioContainer string

// Audio-only files.
const (
	// AudioContainerAuto (the default) follows the codec: .flac for FLAC,
	// .m4a for ALAC, .mp3 otherwise (auto audio is then MP3).
	AudioContainerAuto AudioContainer = "auto"
	// AudioContainerMP3 is audio.mp3 (audio/mpeg); it holds MP3 only.
	AudioContainerMP3 AudioContainer = "mp3"
	// AudioContainerFLAC is audio.flac (audio/flac); it holds FLAC only.
	AudioContainerFLAC AudioContainer = "flac"
	// AudioContainerM4A is audio.m4a (audio/mp4); it holds any codec (auto
	// audio in an .m4a is Opus).
	AudioContainerM4A AudioContainer = "m4a"
)

// HeAac is what an HE-AAC (or HE-AAC v2) source becomes. HE-AAC is decoded
// only as its AAC-LC core: spectral band replication and parametric stereo
// are not decoded, so the core has half the stream's rate, less bandwidth
// and, for v2, one channel. AAC-LC sources are decoded in full whatever it
// says. Values added later decode as they are.
type HeAac string

// HE-AAC policies.
const (
	// HeAacAuto (the default) passes an HE-AAC source through when only a
	// codec change is asked, and decodes its core when the job needs PCM (a
	// downmix, an .mp3 or .flac file).
	HeAacAuto HeAac = "auto"
	// HeAacPassthrough never decodes it: a job that would need it decoded
	// fails.
	HeAacPassthrough HeAac = "passthrough"
	// HeAacCore decodes its core whenever another codec is asked.
	HeAacCore HeAac = "core"
)

// AudioChannels is an audio channel layout. The layouts other than
// ChannelsSource downmix and never upmix. Values added later decode as they
// are.
type AudioChannels string

// Audio channel layouts.
const (
	// ChannelsSource keeps the source's layout (the default).
	ChannelsSource AudioChannels = "source"
	ChannelsMono   AudioChannels = "mono"
	ChannelsStereo AudioChannels = "stereo"
	// ChannelsSurround51 is 5.1 surround; not with MP3.
	ChannelsSurround51 AudioChannels = "5.1"
	// ChannelsSurround71 is 7.1 surround; not with MP3.
	ChannelsSurround71 AudioChannels = "7.1"
)

// Audio handling.
type Audio struct {
	// Mode is auto, opus, mp3, aac, flac, alac or drop (the AudioMode
	// constants).
	Mode string `json:"mode,omitempty"`
	// Bitrate is a bitrate such as "128k" (6k–512k). MP3 takes 32k, 40k, 48k,
	// 56k, 64k, 80k, 96k, 112k, 128k, 160k, 192k, 224k, 256k or 320k (default
	// 128k stereo, 64k mono). AAC takes 8k to 288k per main channel (the LFE
	// does not count; default 64k mono, 128k stereo, 384k 5.1, 512k 7.1).
	// Not with FLAC or ALAC.
	Bitrate *string `json:"bitrate,omitempty"`
	// Channels is the channel layout; left empty, the source's.
	Channels AudioChannels `json:"channels,omitempty"`
	// StereoFallback, in HLS with surround audio, also adds a stereo
	// rendition to the same audio group. The API's default is false.
	StereoFallback *bool `json:"stereo_fallback,omitempty"`
	// BitDepth, for FLAC and ALAC only, is the output's sample depth.
	BitDepth AudioBitDepth `json:"bit_depth,omitempty"`
	// FlacCompression, for FLAC only, is the compression effort.
	FlacCompression FlacCompression `json:"flac_compression,omitempty"`
	// Container, for ModeAudio only, is the file the output is; left empty,
	// auto.
	Container AudioContainer `json:"container,omitempty"`
	// HeAac is what an HE-AAC source becomes; left empty, auto. Not with
	// AudioModeDrop.
	HeAac HeAac `json:"he_aac,omitempty"`
}

// Trim cuts the input to [Start, End) seconds.
type Trim struct {
	Start float64  `json:"start"`
	End   *float64 `json:"end,omitempty"`
}

// OutputSpec is a fully resolved output specification, as returned on jobs
// and presets.
type OutputSpec struct {
	// Mode is "single" (one MP4 per rendition), "hls" or "audio" (one .mp3,
	// .flac or .m4a).
	Mode string `json:"mode"`
	// Codec is av1, h264 or h265.
	Codec      string      `json:"codec"`
	Renditions []Rendition `json:"renditions"`
	// Fit is how the video meets each rendition's box (a Fit* constant).
	Fit string `json:"fit"`
	// Upscale lets a rendition be larger than the source.
	Upscale bool    `json:"upscale"`
	Ladder  *Ladder `json:"ladder"`
	Quality Quality `json:"quality"`
	// Gop is in frames; nil for the default of 2 s.
	Gop            *int     `json:"gop"`
	SegmentSeconds *float64 `json:"segment_seconds"`
	Audio          Audio    `json:"audio"`
	// Subtitles is "all", "none" or a list such as "eng,deu".
	Subtitles *string `json:"subtitles"`
	// Color is sdr, hdr10, hlg or passthrough.
	Color string `json:"color"`
	// BitDepth is auto, 8bit or 10bit.
	BitDepth string   `json:"bit_depth"`
	MaxFPS   *float64 `json:"max_fps"`
	Filters  *string  `json:"filters"`
	Trim     *Trim    `json:"trim"`

	raw json.RawMessage
}

// Raw is the JSON the spec was decoded from, including any fields newer than
// this SDK.
func (s *OutputSpec) Raw() json.RawMessage { return s.raw }

// UnmarshalJSON decodes the spec and keeps its raw JSON.
func (s *OutputSpec) UnmarshalJSON(b []byte) error {
	type plain OutputSpec
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*s = OutputSpec(p)
	s.raw = append(json.RawMessage(nil), b...)
	return nil
}

// OutputSpecInput is a partial output specification: job output overrides
// (merged over the preset: objects merge, arrays replace, null clears), a
// preset's output, or an automation's overrides. Fields left zero are not
// sent; Nullable fields can also be sent as null.
type OutputSpecInput struct {
	Mode           string            `json:"mode,omitempty"`
	Codec          string            `json:"codec,omitempty"`
	Renditions     []Rendition       `json:"renditions,omitempty"`
	Fit            string            `json:"fit,omitempty"`
	Upscale        *bool             `json:"upscale,omitempty"`
	Ladder         Nullable[Ladder]  `json:"ladder,omitzero"`
	Quality        *Quality          `json:"quality,omitempty"`
	Gop            Nullable[int]     `json:"gop,omitzero"`
	SegmentSeconds Nullable[float64] `json:"segment_seconds,omitzero"`
	Audio          *Audio            `json:"audio,omitempty"`
	Subtitles      Nullable[string]  `json:"subtitles,omitzero"`
	Color          string            `json:"color,omitempty"`
	BitDepth       string            `json:"bit_depth,omitempty"`
	MaxFPS         Nullable[float64] `json:"max_fps,omitzero"`
	Filters        Nullable[string]  `json:"filters,omitzero"`
	Trim           Nullable[Trim]    `json:"trim,omitzero"`

	verbatim json.RawMessage // sent as is (RawOutputSpec)
	raw      json.RawMessage // as decoded
}

// RawOutputSpec is an output specification given as JSON, sent exactly as is:
// for specs built elsewhere, or fields newer than this SDK.
func RawOutputSpec(spec json.RawMessage) *OutputSpecInput {
	return &OutputSpecInput{verbatim: append(json.RawMessage(nil), spec...)}
}

// Raw is the JSON the spec was decoded from or built with ([RawOutputSpec]).
func (s *OutputSpecInput) Raw() json.RawMessage {
	if s.verbatim != nil {
		return s.verbatim
	}
	return s.raw
}

// MarshalJSON sends a [RawOutputSpec] as is, else the fields that are set.
func (s OutputSpecInput) MarshalJSON() ([]byte, error) {
	if s.verbatim != nil {
		return s.verbatim, nil
	}
	type plain OutputSpecInput
	return json.Marshal(plain(s))
}

// UnmarshalJSON decodes the spec and keeps its raw JSON.
func (s *OutputSpecInput) UnmarshalJSON(b []byte) error {
	type plain OutputSpecInput
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*s = OutputSpecInput(p)
	s.raw = append(json.RawMessage(nil), b...)
	return nil
}

// AudioStream is one audio stream of a probed input.
type AudioStream struct {
	Codec      string  `json:"codec"`
	Channels   int     `json:"channels"`
	SampleRate int     `json:"sample_rate"`
	Language   *string `json:"language"`
}

// SubtitleStream is one subtitle stream of a probed input.
type SubtitleStream struct {
	Format   string  `json:"format"`
	Language *string `json:"language"`
}

// MediaInfo is what probing an input found.
type MediaInfo struct {
	Container  string  `json:"container"`
	VideoCodec string  `json:"video_codec"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FrameRate  float64 `json:"frame_rate"`
	// Duration is in seconds.
	Duration    float64          `json:"duration"`
	PixelFormat string           `json:"pixel_format"`
	BitDepth    int              `json:"bit_depth"`
	HDR         bool             `json:"hdr"`
	Rotation    int              `json:"rotation"`
	Audio       []AudioStream    `json:"audio"`
	Subtitles   []SubtitleStream `json:"subtitles"`
	SizeBytes   int64            `json:"size_bytes"`
	// DisplayWidth and DisplayHeight are set for non-square pixels only: the
	// size the picture is shown at (720x576 at 64:45 is shown 1024x576).
	DisplayWidth  int `json:"display_width,omitempty"`
	DisplayHeight int `json:"display_height,omitempty"`
}

// SignedURL is a short-lived download URL.
type SignedURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
