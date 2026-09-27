package transcdr

import (
	"encoding/json"
	"time"
)

// Metadata is string-to-string metadata: up to 20 keys (≤ 40 characters)
// with values of at most 500 characters.
type Metadata map[string]string

// Rendition is one rung of the output ladder.
type Rendition struct {
	// Width is even, 64–7680.
	Width int `json:"width"`
	// Height is even, 64–4320.
	Height int `json:"height"`
	// Bitrate is this rendition's constant rate, such as "3M" or "800k"
	// (100k–200M), with quality target "cbr" only.
	Bitrate *string `json:"bitrate,omitempty"`
	// Label is 1–32 of [A-Za-z0-9_-]; the API's default is "<short side>p".
	Label *string `json:"label,omitempty"`
}

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

// Audio handling.
type Audio struct {
	// Mode is auto, opus or drop.
	Mode string `json:"mode,omitempty"`
	// Bitrate is an Opus bitrate such as "128k" (6k–512k).
	Bitrate *string `json:"bitrate,omitempty"`
}

// Trim cuts the input to [Start, End) seconds.
type Trim struct {
	Start float64  `json:"start"`
	End   *float64 `json:"end,omitempty"`
}

// OutputSpec is a fully resolved output specification, as returned on jobs
// and presets.
type OutputSpec struct {
	// Mode is "single" (one MP4 per rendition) or "hls".
	Mode string `json:"mode"`
	// Codec is av1, h264 or h265.
	Codec      string      `json:"codec"`
	Renditions []Rendition `json:"renditions"`
	Ladder     *Ladder     `json:"ladder"`
	Quality    Quality     `json:"quality"`
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
