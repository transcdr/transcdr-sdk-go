package transcdr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Metadata is string-to-string metadata: up to 20 keys (≤ 40 characters)
// with values of at most 500 characters.
type Metadata map[string]string

// OutputKind is what a job produces (OutputSpec.Kind).
type OutputKind string

// Output kinds.
const (
	// KindVideo is video: one MP4 per size, or an HLS package.
	KindVideo OutputKind = "video"
	// KindAudio is the audio alone, as one file.
	KindAudio OutputKind = "audio"
	// KindImage is still images, of an image input or taken from a video.
	KindImage OutputKind = "image"
)

// OutputSpec is an output specification (v2): what a job produces, declared
// in sections. Nothing has a default: a spec states every field its kind,
// container, codec and audio handling need, and a value that follows the
// source is written out ("source", "standard", "from_color", "by_size",
// "poster", "segment", "all").
//
// Build one with [NewVideoOutput], [NewAudioOutput] or [NewImageOutput],
// which take every section the kind needs. [ValidateOutput] reports every
// missing or misplaced field at once, as the API's 422 does; the SDK runs it
// before sending a whole spec.
//
// Jobs, presets and automations return the spec fully resolved.
type OutputSpec struct {
	Kind OutputKind `json:"kind,omitempty"`
	// Container is the file or package; kind video and audio.
	Container *Container `json:"container,omitempty"`
	// Video is the video track; kind video.
	Video *Video `json:"video,omitempty"`
	// Audio is the audio track; kind video and audio.
	Audio *Audio `json:"audio,omitempty"`
	// Image is the still images; kind image.
	Image *Image `json:"image,omitempty"`
	// Renditions are the sizes produced; kind video and image.
	Renditions *Renditions `json:"renditions,omitempty"`
	// Subtitles are the subtitle tracks carried; kind video.
	Subtitles *Subtitles `json:"subtitles,omitempty"`
	// Trim is the part of the source used; kind video.
	Trim *Trim `json:"trim,omitempty"`
	// Privacy is which identifying metadata survives; every kind.
	Privacy Privacy `json:"privacy"`

	raw json.RawMessage
}

// NewVideoOutput is a kind "video" spec with every section it needs.
func NewVideoOutput(container Container, video Video, audio Audio, renditions Renditions, subtitles Subtitles, trim Trim, privacy Privacy) OutputSpec {
	return OutputSpec{
		Kind: KindVideo, Container: &container, Video: &video, Audio: &audio,
		Renditions: &renditions, Subtitles: &subtitles, Trim: &trim, Privacy: privacy,
	}
}

// NewAudioOutput is a kind "audio" spec: one audio file.
func NewAudioOutput(container Container, audio Audio, privacy Privacy) OutputSpec {
	return OutputSpec{Kind: KindAudio, Container: &container, Audio: &audio, Privacy: privacy}
}

// NewImageOutput is a kind "image" spec: every size in every format.
func NewImageOutput(image Image, renditions Renditions, privacy Privacy) OutputSpec {
	return OutputSpec{Kind: KindImage, Image: &image, Renditions: &renditions, Privacy: privacy}
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

// OutputOverrides are fields merged over a preset's spec, in the v2 shape:
// objects merge key by key, scalars and arrays replace, one choice of an
// exclusive group (quality/crf/cbr, sizes/ladder/source_size,
// tracks/languages) replaces the others, and a nil value (JSON null)
// removes a field. The kind cannot change. The merged result must be
// complete, which the API checks.
//
//	transcdr.OutputOverrides{"video": map[string]any{"frame_rate": map[string]any{"max": 24}}}
type OutputOverrides map[string]any

// ContainerFormat is the file or package (Container.Format).
type ContainerFormat string

// Container formats.
const (
	// FormatMP4 is one faststart MP4 per size; kind video.
	FormatMP4 ContainerFormat = "mp4"
	// FormatHLS is a CMAF/HLS package; kind video. Needs SegmentSeconds.
	FormatHLS ContainerFormat = "hls"
	// FormatMP3 is an .mp3, holding MP3 only; kind audio.
	FormatMP3 ContainerFormat = "mp3"
	// FormatFLAC is a .flac, holding FLAC only; kind audio.
	FormatFLAC ContainerFormat = "flac"
	// FormatM4A is an .m4a, holding any audio codec; kind audio.
	FormatM4A ContainerFormat = "m4a"
)

// Container is the file or package.
type Container struct {
	Format ContainerFormat `json:"format,omitempty"`
	// SegmentSeconds is 1–20, with FormatHLS only.
	SegmentSeconds *float64 `json:"segment_seconds,omitempty"`
}

// ContainerMP4 is one MP4 per size.
func ContainerMP4() Container { return Container{Format: FormatMP4} }

// ContainerHLS is an HLS package cut into segments of segmentSeconds (1–20).
func ContainerHLS(segmentSeconds float64) Container {
	return Container{Format: FormatHLS, SegmentSeconds: &segmentSeconds}
}

// ContainerAudio is an audio file: FormatMP3, FormatFLAC or FormatM4A.
func ContainerAudio(format ContainerFormat) Container { return Container{Format: format} }

// Video codecs (Video.Codec).
const (
	CodecAV1  = "av1"
	CodecH264 = "h264"
	// CodecH265 (HEVC) needs a paid plan.
	CodecH265 = "h265"
)

// Quality levels (QualityLevel).
const (
	QualityVisuallyLossless = "visually_lossless"
	QualityHigh             = "high"
	QualityStandard         = "standard"
	QualityLow              = "low"
)

// QualityVMAF is the quality level "vmaf=N" (1–100).
func QualityVMAF(score int) string { return "vmaf=" + itoa(score) }

// BitrateStandard is the bit rate that follows the output: for video, a rate
// per size by codec, short side and frame rate; for audio, by codec and
// channel layout.
const BitrateStandard = "standard"

// CBR codes at a constant bit rate.
type CBR struct {
	// Bitrate is 100k–200M, such as "5M", or BitrateStandard.
	Bitrate string `json:"bitrate"`
	// BufferMs is the rate buffer, 100–10000 ms.
	BufferMs int `json:"buffer_ms"`
}

// VideoRate is how the video is coded: exactly one of a quality level, a
// constant rate factor or a constant bit rate. Build one with QualityLevel,
// ConstantRateFactor or ConstantBitRate.
type VideoRate struct {
	// Quality is a quality level (the Quality* constants or QualityVMAF).
	Quality *string `json:"quality,omitempty"`
	// CRF is 0–63.
	CRF *int `json:"crf,omitempty"`
	CBR *CBR `json:"cbr,omitempty"`
}

// QualityLevel codes to a quality level: QualityVisuallyLossless,
// QualityHigh, QualityStandard, QualityLow or QualityVMAF(n).
func QualityLevel(level string) VideoRate { return VideoRate{Quality: &level} }

// ConstantRateFactor codes to a constant rate factor, 0–63.
func ConstantRateFactor(crf int) VideoRate { return VideoRate{CRF: &crf} }

// ConstantBitRate codes every size at bitrate ("5M", or BitrateStandard),
// held within a buffer of bufferMs (100–10000).
func ConstantBitRate(bitrate string, bufferMs int) VideoRate {
	return VideoRate{CBR: &CBR{Bitrate: bitrate, BufferMs: bufferMs}}
}

// VideoBitDepth is the video's bit depth. Values added later decode as they
// are.
type VideoBitDepth string

// Video bit depths.
const (
	// BitDepthFromColor is 8-bit for sdr, 10-bit for hdr10 and hlg, the
	// source's for passthrough.
	BitDepthFromColor VideoBitDepth = "from_color"
	BitDepth8         VideoBitDepth = "8bit"
	// BitDepth10 is not offered with H.264.
	BitDepth10 VideoBitDepth = "10bit"
)

// Colours (Video.Color).
const (
	// ColorSDR tone-maps an HDR source.
	ColorSDR = "sdr"
	// ColorHDR10 needs a paid plan.
	ColorHDR10 = "hdr10"
	// ColorHLG needs a paid plan.
	ColorHLG         = "hlg"
	ColorPassthrough = "passthrough"
)

// Video is the video track (kind video). VideoRate is embedded: set exactly
// one of its fields.
type Video struct {
	// Codec is CodecAV1, CodecH264 or CodecH265.
	Codec string `json:"codec,omitempty"`
	VideoRate
	BitDepth VideoBitDepth `json:"bit_depth,omitempty"`
	// Color is a Color* constant.
	Color     string    `json:"color,omitempty"`
	FrameRate FrameRate `json:"frame_rate"`
	Gop       Gop       `json:"gop"`
	// Filters is a filter chain, one filter per entry such as
	// "crop=1280:720"; empty for none (sent as []).
	Filters []string `json:"filters"`
}

// NewVideo is a video track with every field it needs.
func NewVideo(codec string, rate VideoRate, bitDepth VideoBitDepth, color string, frameRate FrameRate, gop Gop, filters []string) Video {
	return Video{Codec: codec, VideoRate: rate, BitDepth: bitDepth, Color: color, FrameRate: frameRate, Gop: gop, Filters: filters}
}

// MarshalJSON sends Filters as [] when there are none.
func (v Video) MarshalJSON() ([]byte, error) {
	type plain Video
	p := plain(v)
	if p.Filters == nil {
		p.Filters = []string{}
	}
	return json.Marshal(p)
}

// FrameRate is the video's frame rate (Video.FrameRate). Build it with
// FrameRateSource or MaxFrameRate; the zero value is unset (sent as null).
type FrameRate struct {
	Max FrameRateMax `json:"max"`
}

// FrameRateSource keeps the source's frame rate, not capped.
func FrameRateSource() FrameRate { return FrameRate{Max: FrameRateMax{set: true, source: true}} }

// MaxFrameRate caps the frame rate at fps (1–240).
func MaxFrameRate(fps float64) FrameRate { return FrameRate{Max: FrameRateMax{set: true, fps: fps}} }

// FrameRateMax is a frame rate cap, or "source".
type FrameRateMax struct {
	fps    float64
	source bool
	set    bool
}

// FPS is the cap, and false for "source" or unset.
func (m FrameRateMax) FPS() (float64, bool) { return m.fps, m.set && !m.source }

// IsSource reports "source".
func (m FrameRateMax) IsSource() bool { return m.source }

// MarshalJSON writes the number, "source", or null when unset.
func (m FrameRateMax) MarshalJSON() ([]byte, error) {
	return marshalNumberOr(m.set, m.source, m.fps, "source")
}

// UnmarshalJSON reads a number or "source".
func (m *FrameRateMax) UnmarshalJSON(b []byte) error {
	set, word, n, err := unmarshalNumberOr(b, "source")
	*m = FrameRateMax{set: set, source: word, fps: n}
	return err
}

// Gop is the keyframe interval: GopFrames, GopSeconds, or (HLS) GopSegment.
// The zero value is unset (sent as null).
type Gop struct {
	frames  *int
	seconds *float64
	segment bool
}

// GopFrames is a keyframe every n frames (1–1200).
func GopFrames(n int) Gop { return Gop{frames: &n} }

// GopSeconds is a keyframe every s seconds (0.1–60).
func GopSeconds(s float64) Gop { return Gop{seconds: &s} }

// GopSegment is, for HLS, one keyframe at the start of each segment and none
// inside it.
func GopSegment() Gop { return Gop{segment: true} }

// Frames is the interval in frames, if it is one.
func (g Gop) Frames() (int, bool) {
	if g.frames == nil {
		return 0, false
	}
	return *g.frames, true
}

// Seconds is the interval in seconds, if it is one.
func (g Gop) Seconds() (float64, bool) {
	if g.seconds == nil {
		return 0, false
	}
	return *g.seconds, true
}

// IsSegment reports "segment".
func (g Gop) IsSegment() bool { return g.segment }

// MarshalJSON writes {"frames": N}, {"seconds": N}, "segment", or null.
func (g Gop) MarshalJSON() ([]byte, error) {
	switch {
	case g.segment:
		return []byte(`"segment"`), nil
	case g.frames != nil:
		return json.Marshal(map[string]int{"frames": *g.frames})
	case g.seconds != nil:
		return json.Marshal(map[string]float64{"seconds": *g.seconds})
	}
	return []byte("null"), nil
}

// UnmarshalJSON reads any of its forms.
func (g *Gop) UnmarshalJSON(b []byte) error {
	*g = Gop{}
	if isNull(b) {
		return nil
	}
	var word string
	if json.Unmarshal(b, &word) == nil {
		if word != "segment" {
			return fmt.Errorf("transcdr: unknown gop %q", word)
		}
		g.segment = true
		return nil
	}
	var o struct {
		Frames  *int     `json:"frames"`
		Seconds *float64 `json:"seconds"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	g.frames, g.seconds = o.Frames, o.Seconds
	return nil
}

// AudioHandling is what happens to the audio (Audio.Handling).
type AudioHandling string

// Audio handling.
const (
	// HandlingAuto keeps the source's audio where the container can carry it
	// unchanged, otherwise makes it Audio.Codec.
	HandlingAuto AudioHandling = "auto"
	// HandlingEncode makes it Audio.Codec (a source already in that codec,
	// with nothing else changed, is copied).
	HandlingEncode AudioHandling = "encode"
	// HandlingDrop makes no audio track; kind video only.
	HandlingDrop AudioHandling = "drop"
)

// AudioCodec is an audio codec (Audio.Codec). Values added later decode as
// they are.
type AudioCodec string

// Audio codecs.
const (
	AudioCodecOpus AudioCodec = "opus"
	// AudioCodecMP3 is constant bit rate MP3, stereo at most; not in HLS.
	AudioCodecMP3 AudioCodec = "mp3"
	// AudioCodecAAC is AAC-LC, the audio that plays on the most devices.
	AudioCodecAAC AudioCodec = "aac"
	// AudioCodecFLAC is lossless; needs BitDepth and FlacCompression.
	AudioCodecFLAC AudioCodec = "flac"
	// AudioCodecALAC is lossless Apple Lossless; needs BitDepth.
	AudioCodecALAC AudioCodec = "alac"
)

// AudioBitDepth is the sample depth of FLAC and ALAC output. Values added
// later decode as they are.
type AudioBitDepth string

// Lossless sample depths.
const (
	// AudioBitDepthSource is 16-bit for a 16-bit or lossy source, 24-bit for a
	// deeper one.
	AudioBitDepthSource AudioBitDepth = "source"
	AudioBitDepth16     AudioBitDepth = "16"
	AudioBitDepth24     AudioBitDepth = "24"
)

// FlacCompression is FLAC's compression effort: the same audio either way,
// a smaller file for more work. Values added later decode as they are.
type FlacCompression string

// FLAC compression efforts.
const (
	FlacCompressionFast     FlacCompression = "fast"
	FlacCompressionBalanced FlacCompression = "balanced"
	FlacCompressionBest     FlacCompression = "best"
)

// HeAac is what an HE-AAC (or HE-AAC v2) source becomes. HE-AAC is decoded
// only as its AAC-LC core: half the sample rate, less bandwidth and, for v2,
// one channel. AAC-LC sources are decoded in full whatever it says. Values
// added later decode as they are.
type HeAac string

// HE-AAC policies.
const (
	// HeAacAuto passes an HE-AAC source through when only a codec change is
	// asked, and decodes its core when the job needs PCM (a downmix, an .mp3
	// or .flac file).
	HeAacAuto HeAac = "auto"
	// HeAacPassthrough never decodes it: a job that would need it decoded
	// fails.
	HeAacPassthrough HeAac = "passthrough"
	// HeAacCore decodes its core whenever another codec or a change is asked.
	HeAacCore HeAac = "core"
)

// AudioChannels is an audio channel layout. The layouts other than
// ChannelsSource downmix and never upmix. Values added later decode as they
// are.
type AudioChannels string

// Audio channel layouts.
const (
	// ChannelsSource keeps the source's layout (MP3 folds a wider one to
	// stereo).
	ChannelsSource AudioChannels = "source"
	ChannelsMono   AudioChannels = "mono"
	ChannelsStereo AudioChannels = "stereo"
	// ChannelsSurround51 is 5.1 surround; not with MP3.
	ChannelsSurround51 AudioChannels = "5.1"
	// ChannelsSurround71 is 7.1 surround; not with MP3.
	ChannelsSurround71 AudioChannels = "7.1"
)

// Audio is the audio track (kind video and audio). With HandlingDrop it has
// no other field; otherwise Codec, Channels and HeAac are required, and the
// rest as the codec and container need:
//   - Bitrate with opus, mp3 and aac;
//   - StereoFallback in HLS;
//   - BitDepth with flac and alac;
//   - FlacCompression with flac.
type Audio struct {
	Handling AudioHandling `json:"handling,omitempty"`
	Codec    AudioCodec    `json:"codec,omitempty"`
	// Bitrate is a rate such as "128k" (6k–512k; MP3 at its fixed rates; AAC
	// 8k to 288k per main channel), or BitrateStandard.
	Bitrate  string        `json:"bitrate,omitempty"`
	Channels AudioChannels `json:"channels,omitempty"`
	HeAac    HeAac         `json:"he_aac,omitempty"`
	// StereoFallback adds, in HLS beside surround audio, a stereo downmix in
	// the same audio group. True needs Channels source, 5.1 or 7.1.
	StereoFallback  *bool           `json:"stereo_fallback,omitempty"`
	BitDepth        AudioBitDepth   `json:"bit_depth,omitempty"`
	FlacCompression FlacCompression `json:"flac_compression,omitempty"`
}

// AudioDrop is no audio track.
func AudioDrop() Audio { return Audio{Handling: HandlingDrop} }

// NewAudio is an audio track (HandlingAuto or HandlingEncode) with the fields
// every track needs. Set the fields its codec and container need too.
func NewAudio(handling AudioHandling, codec AudioCodec, channels AudioChannels, heAac HeAac) Audio {
	return Audio{Handling: handling, Codec: codec, Channels: channels, HeAac: heAac}
}

// ImageFormat is an image output format. Values added later decode as they
// are.
type ImageFormat string

// Image output formats (Image.Formats).
const (
	// ImageFormatAVIF is lossy and the smallest.
	ImageFormatAVIF ImageFormat = "avif"
	// ImageFormatWebP is lossy unless Image.Lossless.
	ImageFormatWebP ImageFormat = "webp"
	ImageFormatJPEG ImageFormat = "jpeg"
	// ImageFormatPNG is always lossless.
	ImageFormatPNG ImageFormat = "png"
)

// Colour profiles (Image.ColorProfile).
const (
	// ColorProfileSRGB converts the pixels to sRGB.
	ColorProfileSRGB = "srgb"
	// ColorProfileKeep keeps the source's profile (PNG, JPEG, WebP).
	ColorProfileKeep = "keep"
)

// Image is the still images (kind image): every size in every format.
type Image struct {
	// Formats are 1–4 distinct formats.
	Formats []ImageFormat `json:"formats"`
	// Lossless is required when Formats has webp: lossless WebP.
	Lossless *bool `json:"lossless,omitempty"`
	// Quality is 1–100 for each lossy format made (avif, jpeg, and webp
	// unless lossless) and no others, such as {avif: 60, jpeg: 82}.
	Quality map[ImageFormat]int `json:"quality,omitempty"`
	// ColorProfile is ColorProfileSRGB or ColorProfileKeep.
	ColorProfile string      `json:"color_profile,omitempty"`
	Frames       ImageFrames `json:"frames"`
}

// NewImage is the image section with the fields it always needs. Set
// Quality and Lossless as its formats need.
func NewImage(formats []ImageFormat, colorProfile string, frames ImageFrames) Image {
	return Image{Formats: formats, ColorProfile: colorProfile, Frames: frames}
}

// ImageFrames picks the stills: FramesPoster, FramesCount or FramesAt. The
// zero value is unset (sent as null).
type ImageFrames struct {
	count     *int
	atSeconds []float64
	poster    bool
}

// FramesPoster is an image input as it is, or a video's frame 10% of the way
// in.
func FramesPoster() ImageFrames { return ImageFrames{poster: true} }

// FramesCount is n stills (1–100) evenly spaced through a video input.
func FramesCount(n int) ImageFrames { return ImageFrames{count: &n} }

// FramesAt is stills at these seconds of a video input (1–100 of them).
func FramesAt(seconds ...float64) ImageFrames {
	return ImageFrames{atSeconds: append([]float64{}, seconds...)}
}

// IsPoster reports "poster".
func (f ImageFrames) IsPoster() bool { return f.poster }

// Count is the number of stills, if given as a count.
func (f ImageFrames) Count() (int, bool) {
	if f.count == nil {
		return 0, false
	}
	return *f.count, true
}

// AtSeconds are the stills' times, if given as times.
func (f ImageFrames) AtSeconds() []float64 { return f.atSeconds }

// MarshalJSON writes "poster", {"count": N}, {"at_seconds": [...]}, or null.
func (f ImageFrames) MarshalJSON() ([]byte, error) {
	switch {
	case f.poster:
		return []byte(`"poster"`), nil
	case f.count != nil:
		return json.Marshal(map[string]int{"count": *f.count})
	case f.atSeconds != nil:
		return json.Marshal(map[string][]float64{"at_seconds": f.atSeconds})
	}
	return []byte("null"), nil
}

// UnmarshalJSON reads any of its forms.
func (f *ImageFrames) UnmarshalJSON(b []byte) error {
	*f = ImageFrames{}
	if isNull(b) {
		return nil
	}
	var word string
	if json.Unmarshal(b, &word) == nil {
		if word != "poster" {
			return fmt.Errorf("transcdr: unknown frames %q", word)
		}
		f.poster = true
		return nil
	}
	var o struct {
		Count     *int      `json:"count"`
		AtSeconds []float64 `json:"at_seconds"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	f.count, f.atSeconds = o.Count, o.AtSeconds
	return nil
}

// How the picture meets a size's box (Size.Fit, Ladder.Fit, SourceSize.Fit).
const (
	// FitContain keeps the picture's shape inside the box.
	FitContain = "contain"
	// FitCover fills the box, keeping the shape, and centre-crops the rest.
	FitCover = "cover"
	// FitPad keeps the shape and adds black bars to exactly the box.
	FitPad = "pad"
	// FitStretch distorts the picture to exactly the box.
	FitStretch = "stretch"
)

// Whether a size's box turns to the picture (Size.Orientation).
const (
	// OrientationAuto turns the box to the picture's orientation: 1920x1080
	// on a portrait video is 1080x1920.
	OrientationAuto = "auto"
	// OrientationFixed uses the box as written.
	OrientationFixed = "fixed"
)

// LabelBySize names an output by the size it comes out at: "<short side>p"
// for video, "<width>x<height>" for images.
const LabelBySize = "by_size"

// Renditions are the sizes produced (kind video and image): exactly one of
// Sizes, Ladder (video only) and SourceSize. Build them with
// RenditionSizes, RenditionLadder or RenditionSourceSize.
type Renditions struct {
	Sizes      []Size      `json:"sizes,omitempty"`
	Ladder     *Ladder     `json:"ladder,omitempty"`
	SourceSize *SourceSize `json:"source_size,omitempty"`
}

// RenditionSizes are explicit sizes, each a maximum box.
func RenditionSizes(sizes ...Size) Renditions { return Renditions{Sizes: sizes} }

// RenditionLadder is an automatic ladder of standard short sides up to
// maxShortSide, never above the source (video only).
func RenditionLadder(maxShortSide int, fit string, upscale bool) Renditions {
	return Renditions{Ladder: &Ladder{MaxShortSide: maxShortSide, Fit: fit, Upscale: upscale}}
}

// RenditionSourceSize is one output at the source's size.
func RenditionSourceSize(label, fit string, upscale bool) Renditions {
	return Renditions{SourceSize: &SourceSize{Label: label, Fit: fit, Upscale: upscale}}
}

// Size is one output size. Width x Height is the largest the output may be:
// the picture keeps its shape inside that box (see Fit) and is not enlarged
// past its own size unless Upscale is on. Each output reports the size it
// came out at.
type Size struct {
	// Label is 1–32 of [A-Za-z0-9_-], or LabelBySize.
	Label string `json:"label,omitempty"`
	// Width is even, 64–7680 for video; 16–8192 for images.
	Width int `json:"width"`
	// Height is even, 64–4320 for video; 16–8192 for images.
	Height int `json:"height"`
	// Fit is a Fit* constant.
	Fit string `json:"fit,omitempty"`
	// Orientation is OrientationAuto or OrientationFixed.
	Orientation string `json:"orientation,omitempty"`
	Upscale     bool   `json:"upscale"`
	// Video is this size's own constant rate, with Video.CBR only; left
	// nil, the size uses Video.CBR.Bitrate.
	Video *SizeVideo `json:"video,omitempty"`
}

// NewSize is a size with every field it needs.
func NewSize(label string, width, height int, fit, orientation string, upscale bool) Size {
	return Size{Label: label, Width: width, Height: height, Fit: fit, Orientation: orientation, Upscale: upscale}
}

// WithBitrate is the size with its own constant rate (with Video.CBR only).
func (s Size) WithBitrate(bitrate string) Size {
	s.Video = &SizeVideo{CBR: SizeCBR{Bitrate: bitrate}}
	return s
}

// SizeVideo is a size's own video settings.
type SizeVideo struct {
	CBR SizeCBR `json:"cbr"`
}

// SizeCBR is a size's own constant rate.
type SizeCBR struct {
	// Bitrate is 100k–200M, or BitrateStandard.
	Bitrate string `json:"bitrate"`
}

// Ladder is an automatic ABR ladder (video only).
type Ladder struct {
	// MaxShortSide is the top rung's short side (64 up to the plan's maximum).
	MaxShortSide int    `json:"max_short_side"`
	Fit          string `json:"fit,omitempty"`
	Upscale      bool   `json:"upscale"`
}

// SourceSize is one output at the source's size.
type SourceSize struct {
	// Label is 1–32 of [A-Za-z0-9_-], or LabelBySize.
	Label   string `json:"label,omitempty"`
	Fit     string `json:"fit,omitempty"`
	Upscale bool   `json:"upscale"`
}

// Subtitle track choices (Subtitles.Tracks).
const (
	// SubtitleTracksAll carries every subtitle track of the source.
	SubtitleTracksAll = "all"
	// SubtitleTracksNone carries none.
	SubtitleTracksNone = "none"
)

// Subtitles are the subtitle tracks carried (kind video): exactly one of
// Tracks and Languages.
type Subtitles struct {
	// Tracks is SubtitleTracksAll or SubtitleTracksNone.
	Tracks string `json:"tracks,omitempty"`
	// Languages are ISO 639-2 codes such as "eng", one or more.
	Languages []string `json:"languages,omitempty"`
}

// SubtitlesAll carries every subtitle track.
func SubtitlesAll() Subtitles { return Subtitles{Tracks: SubtitleTracksAll} }

// SubtitlesNone carries no subtitles.
func SubtitlesNone() Subtitles { return Subtitles{Tracks: SubtitleTracksNone} }

// SubtitleLanguages carries the tracks in these languages.
func SubtitleLanguages(codes ...string) Subtitles { return Subtitles{Languages: codes} }

// Trim is the part of the source used (kind video), in seconds.
type Trim struct {
	// Start is ≥ 0.
	Start float64 `json:"start"`
	// End is after Start, or TrimEndSource.
	End TrimEnd `json:"end"`
}

// NewTrim is the part of the source from start to end.
func NewTrim(start float64, end TrimEnd) Trim { return Trim{Start: start, End: end} }

// TrimEnd is where a trim ends: TrimEndAt or TrimEndSource. The zero value
// is unset (sent as null).
type TrimEnd struct {
	seconds float64
	source  bool
	set     bool
}

// TrimEndSource is the end of the source.
func TrimEndSource() TrimEnd { return TrimEnd{set: true, source: true} }

// TrimEndAt is seconds from the start.
func TrimEndAt(seconds float64) TrimEnd { return TrimEnd{set: true, seconds: seconds} }

// Seconds is the end in seconds, and false for "source" or unset.
func (e TrimEnd) Seconds() (float64, bool) { return e.seconds, e.set && !e.source }

// IsSource reports "source".
func (e TrimEnd) IsSource() bool { return e.source }

// MarshalJSON writes the number, "source", or null when unset.
func (e TrimEnd) MarshalJSON() ([]byte, error) {
	return marshalNumberOr(e.set, e.source, e.seconds, "source")
}

// UnmarshalJSON reads a number or "source".
func (e *TrimEnd) UnmarshalJSON(b []byte) error {
	set, word, n, err := unmarshalNumberOr(b, "source")
	*e = TrimEnd{set: set, source: word, seconds: n}
	return err
}

// Privacy presets (Privacy.Preset).
const (
	// PrivacyStripAll strips every category.
	PrivacyStripAll = "strip_all"
	// PrivacyStripLocation strips the location and keeps the rest.
	PrivacyStripLocation = "strip_location"
	// PrivacyKeepAll keeps everything, serial numbers and owner too.
	PrivacyKeepAll = "keep_all"
)

// Privacy is which identifying metadata of the source survives: a Preset,
// whose categories any of the four fields refine, or all four categories
// without a preset. Build it with PrivacyPreset (refined with its With…
// methods) or PrivacyFields. Responses always give the four categories.
//
//	transcdr.PrivacyPreset(transcdr.PrivacyStripAll).WithCaptureTime("date")
type Privacy struct {
	// Preset is PrivacyStripAll, PrivacyStripLocation or PrivacyKeepAll.
	Preset string `json:"preset,omitempty"`
	// Location is "strip", "approximate" (about 1 km) or "keep".
	Location string `json:"location,omitempty"`
	// CaptureTime is "strip", "date" or "keep".
	CaptureTime string `json:"capture_time,omitempty"`
	// Device is "strip", "keep" or "keep_all" (serial numbers and owner too).
	Device string `json:"device,omitempty"`
	// Descriptive is "strip" or "keep": title, artist, copyright and so on.
	Descriptive string `json:"descriptive,omitempty"`
}

// PrivacyPreset is one of the privacy presets.
func PrivacyPreset(preset string) Privacy { return Privacy{Preset: preset} }

// WithLocation refines the preset's location handling.
func (p Privacy) WithLocation(location string) Privacy { p.Location = location; return p }

// WithCaptureTime refines the preset's capture time handling.
func (p Privacy) WithCaptureTime(captureTime string) Privacy { p.CaptureTime = captureTime; return p }

// WithDevice refines the preset's device handling.
func (p Privacy) WithDevice(device string) Privacy { p.Device = device; return p }

// WithDescriptive refines the preset's descriptive tags handling.
func (p Privacy) WithDescriptive(descriptive string) Privacy { p.Descriptive = descriptive; return p }

// PrivacyFields states each category, without a preset.
func PrivacyFields(location, captureTime, device, descriptive string) Privacy {
	return Privacy{Location: location, CaptureTime: captureTime, Device: device, Descriptive: descriptive}
}

// PresetProvenance is where a job's spec came from: the preset as the
// request named it, the version used, and the request's output over it.
type PresetProvenance struct {
	// ID is the preset as the request named it (a slug or pre_… id).
	ID string `json:"id"`
	// Slug is the preset's slug.
	Slug    string `json:"slug"`
	Version int    `json:"version"`
	// Overrides are the request's output over the preset, in v2; nil when
	// none.
	Overrides OutputOverrides `json:"overrides"`
}

// PresetVersion is one version of a preset: a complete spec that never
// changes.
type PresetVersion struct {
	Version   int        `json:"version"`
	Output    OutputSpec `json:"output"`
	CreatedAt *time.Time `json:"created_at"`
}

// AudioStream is one audio stream of a probed input.
type AudioStream struct {
	Codec      string  `json:"codec,omitempty"`
	Channels   int     `json:"channels"`
	SampleRate int     `json:"sample_rate"`
	Language   *string `json:"language"`
}

// SubtitleStream is one subtitle stream of a probed input.
type SubtitleStream struct {
	Format   string  `json:"format,omitempty"`
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
	BitDepth    int              `json:"bit_depth,omitempty"`
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

func isNull(b []byte) bool { return bytes.Equal(bytes.TrimSpace(b), []byte("null")) }

func marshalNumberOr(set, word bool, n float64, w string) ([]byte, error) {
	switch {
	case !set:
		return []byte("null"), nil
	case word:
		return json.Marshal(w)
	}
	return json.Marshal(n)
}

func unmarshalNumberOr(b []byte, w string) (set, word bool, n float64, err error) {
	if isNull(b) {
		return false, false, 0, nil
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		if s != w {
			return false, false, 0, errors.New("transcdr: expected a number or " + strconvQuote(w))
		}
		return true, true, 0, nil
	}
	if err := json.Unmarshal(b, &n); err != nil {
		return false, false, 0, err
	}
	return true, false, n, nil
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
