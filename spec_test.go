package transcdr

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// sameJSON fails unless got marshals to the same JSON value as want.
func sameJSON(t *testing.T, got any, want string) {
	t.Helper()
	b, err := json.Marshal(got)
	must(t, err)
	var g, w any
	must(t, json.Unmarshal(b, &g))
	must(t, json.Unmarshal([]byte(want), &w))
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("marshal = %s\nwant      %s", b, want)
	}
}

// The complete specs of the API's documented examples.
func exampleHLS() OutputSpec {
	return NewVideoOutput(
		ContainerHLS(6),
		NewVideo(CodecH264, ConstantBitRate(BitrateStandard, 1000), BitDepth8, ColorSDR, FrameRateSource(), GopSegment(), []string{}),
		Audio{Handling: HandlingEncode, Codec: AudioCodecAAC, Bitrate: BitrateStandard, Channels: ChannelsSource, HeAac: HeAacAuto, StereoFallback: Bool(false)},
		RenditionSizes(
			NewSize(LabelBySize, 1920, 1080, FitContain, OrientationAuto, false).WithBitrate("5M"),
			NewSize(LabelBySize, 1280, 720, FitContain, OrientationAuto, false).WithBitrate("3M"),
		),
		SubtitlesAll(),
		NewTrim(0, TrimEndSource()),
		PrivacyPreset(PrivacyStripAll),
	)
}

func TestSpecExamples(t *testing.T) {
	cases := []struct {
		name string
		spec OutputSpec
		want string
	}{
		{"hls cbr", exampleHLS(), `{ "kind": "video",
  "container": { "format": "hls", "segment_seconds": 6 },
  "video": { "codec": "h264", "cbr": { "bitrate": "standard", "buffer_ms": 1000 }, "bit_depth": "8bit",
             "color": "sdr", "frame_rate": { "max": "source" }, "gop": "segment", "filters": [] },
  "audio": { "handling": "encode", "codec": "aac", "bitrate": "standard", "channels": "source",
             "he_aac": "auto", "stereo_fallback": false },
  "renditions": { "sizes": [
    { "label": "by_size", "width": 1920, "height": 1080, "fit": "contain", "orientation": "auto", "upscale": false, "video": { "cbr": { "bitrate": "5M" } } },
    { "label": "by_size", "width": 1280, "height": 720,  "fit": "contain", "orientation": "auto", "upscale": false, "video": { "cbr": { "bitrate": "3M" } } } ] },
  "subtitles": { "tracks": "all" }, "trim": { "start": 0, "end": "source" }, "privacy": { "preset": "strip_all" } }`},
		{"single mp4", NewVideoOutput(
			ContainerMP4(),
			NewVideo(CodecH264, QualityLevel(QualityHigh), BitDepthFromColor, ColorSDR, MaxFrameRate(30), GopSeconds(2), nil),
			Audio{Handling: HandlingEncode, Codec: AudioCodecAAC, Bitrate: BitrateStandard, Channels: ChannelsSource, HeAac: HeAacAuto},
			RenditionSizes(NewSize(LabelBySize, 1080, 1920, FitCover, OrientationFixed, false)),
			SubtitlesAll(),
			NewTrim(0, TrimEndSource()),
			PrivacyPreset(PrivacyStripAll),
		), `{ "kind": "video",
  "container": { "format": "mp4" },
  "video": { "codec": "h264", "quality": "high", "bit_depth": "from_color", "color": "sdr",
             "frame_rate": { "max": 30 }, "gop": { "seconds": 2 }, "filters": [] },
  "audio": { "handling": "encode", "codec": "aac", "bitrate": "standard", "channels": "source", "he_aac": "auto" },
  "renditions": { "sizes": [ { "label": "by_size", "width": 1080, "height": 1920, "fit": "cover", "orientation": "fixed", "upscale": false } ] },
  "subtitles": { "tracks": "all" }, "trim": { "start": 0, "end": "source" }, "privacy": { "preset": "strip_all" } }`},
		{"audio mp3", NewAudioOutput(
			ContainerAudio(FormatMP3),
			Audio{Handling: HandlingEncode, Codec: AudioCodecMP3, Bitrate: "64k", Channels: ChannelsMono, HeAac: HeAacAuto},
			PrivacyPreset(PrivacyStripAll),
		), `{ "kind": "audio",
  "container": { "format": "mp3" },
  "audio": { "handling": "encode", "codec": "mp3", "bitrate": "64k", "channels": "mono", "he_aac": "auto" },
  "privacy": { "preset": "strip_all" } }`},
		{"stills", NewImageOutput(
			Image{Formats: []ImageFormat{ImageFormatJPEG}, Quality: map[ImageFormat]int{ImageFormatJPEG: 80}, ColorProfile: ColorProfileSRGB, Frames: FramesCount(12)},
			RenditionSizes(NewSize("sheet", 320, 320, FitContain, OrientationAuto, false)),
			PrivacyPreset(PrivacyStripAll),
		), `{ "kind": "image",
  "image": { "formats": ["jpeg"], "quality": { "jpeg": 80 }, "color_profile": "srgb", "frames": { "count": 12 } },
  "renditions": { "sizes": [ { "label": "sheet", "width": 320, "height": 320, "fit": "contain", "orientation": "auto", "upscale": false } ] },
  "privacy": { "preset": "strip_all" } }`},
		{"ladder, crf, lossless audio, languages", NewVideoOutput(
			ContainerMP4(),
			NewVideo(CodecAV1, ConstantRateFactor(30), BitDepth10, ColorHDR10, FrameRateSource(), GopFrames(48), []string{"hflip"}),
			Audio{Handling: HandlingAuto, Codec: AudioCodecFLAC, Channels: ChannelsStereo, HeAac: HeAacCore, BitDepth: AudioBitDepth24, FlacCompression: FlacCompressionBest},
			RenditionLadder(1080, FitContain, false),
			SubtitleLanguages("eng", "deu"),
			NewTrim(2, TrimEndAt(7.5)),
			PrivacyFields("approximate", "date", "strip", "keep"),
		), `{"kind":"video","container":{"format":"mp4"},
  "video":{"codec":"av1","crf":30,"bit_depth":"10bit","color":"hdr10","frame_rate":{"max":"source"},"gop":{"frames":48},"filters":["hflip"]},
  "audio":{"handling":"auto","codec":"flac","channels":"stereo","he_aac":"core","bit_depth":"24","flac_compression":"best"},
  "renditions":{"ladder":{"max_short_side":1080,"fit":"contain","upscale":false}},
  "subtitles":{"languages":["eng","deu"]},"trim":{"start":2,"end":7.5},
  "privacy":{"location":"approximate","capture_time":"date","device":"strip","descriptive":"keep"}}`},
		{"source size, poster, lossless webp", NewImageOutput(
			Image{Formats: []ImageFormat{ImageFormatWebP, ImageFormatPNG}, Lossless: Bool(true), ColorProfile: ColorProfileKeep, Frames: FramesPoster()},
			RenditionSourceSize(LabelBySize, FitContain, false),
			PrivacyPreset(PrivacyKeepAll),
		), `{"kind":"image","image":{"formats":["webp","png"],"lossless":true,"color_profile":"keep","frames":"poster"},
  "renditions":{"source_size":{"label":"by_size","fit":"contain","upscale":false}},"privacy":{"preset":"keep_all"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sameJSON(t, c.spec, c.want)
			if errs := ValidateOutput(c.spec); errs != nil {
				t.Fatalf("complete spec refused: %+v", errs)
			}
			// It decodes back to the same spec.
			var back OutputSpec
			must(t, json.Unmarshal([]byte(c.want), &back))
			sameJSON(t, back, c.want)
		})
	}
	if QualityVMAF(93) != "vmaf=93" {
		t.Fatal(QualityVMAF(93))
	}
}

func TestSpecChoiceValues(t *testing.T) {
	// Unset values are left out or sent as null, so a missing field is
	// refused rather than defaulted.
	sameJSON(t, Video{}, `{"frame_rate":{"max":null},"gop":null,"filters":[]}`)
	sameJSON(t, Trim{}, `{"start":0,"end":null}`)
	sameJSON(t, Image{}, `{"formats":null,"frames":null}`)
	sameJSON(t, FramesAt(1.5, 10), `{"at_seconds":[1.5,10]}`)

	var v Video
	must(t, json.Unmarshal([]byte(`{"codec":"h264","quality":"vmaf=93","frame_rate":{"max":24},"gop":{"frames":48}}`), &v))
	if fps, ok := v.FrameRate.Max.FPS(); !ok || fps != 24 || v.FrameRate.Max.IsSource() {
		t.Fatalf("frame rate %+v", v.FrameRate)
	}
	if n, ok := v.Gop.Frames(); !ok || n != 48 || v.Gop.IsSegment() || *v.Quality != "vmaf=93" {
		t.Fatalf("video %+v", v)
	}
	must(t, json.Unmarshal([]byte(`{"gop":{"seconds":2.5},"frame_rate":{"max":"source"}}`), &v))
	if s, ok := v.Gop.Seconds(); !ok || s != 2.5 || !v.FrameRate.Max.IsSource() {
		t.Fatalf("video %+v", v)
	}
	var img Image
	must(t, json.Unmarshal([]byte(`{"frames":{"at_seconds":[1,2]}}`), &img))
	if got := img.Frames.AtSeconds(); len(got) != 2 || img.Frames.IsPoster() {
		t.Fatalf("frames %+v", img.Frames)
	}
	must(t, json.Unmarshal([]byte(`{"frames":{"count":3}}`), &img))
	if n, ok := img.Frames.Count(); !ok || n != 3 {
		t.Fatalf("frames %+v", img.Frames)
	}
	var tr Trim
	must(t, json.Unmarshal([]byte(`{"start":1,"end":9}`), &tr))
	if s, ok := tr.End.Seconds(); !ok || s != 9 {
		t.Fatalf("trim %+v", tr)
	}
	for _, bad := range []string{`{"gop":"keyframes"}`, `{"frame_rate":{"max":"fast"}}`} {
		if json.Unmarshal([]byte(bad), &v) == nil {
			t.Fatalf("decoded %s", bad)
		}
	}
	if json.Unmarshal([]byte(`{"frames":"all"}`), &img) == nil {
		t.Fatal("decoded unknown frames")
	}
}

func TestValidateOutputCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/output_validation_cases.json")
	must(t, err)
	var cases []struct {
		Name   string          `json:"name"`
		Output json.RawMessage `json:"output"`
		Errors []FieldError    `json:"errors"`
	}
	must(t, json.Unmarshal(raw, &cases))
	if len(cases) < 20 {
		t.Fatalf("%d cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := ValidateOutputJSON(c.Output)
			if len(got) == 0 && len(c.Errors) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.Errors) {
				t.Fatalf("errors\n got  %+v\n want %+v", got, c.Errors)
			}
		})
	}
	if got := ValidateOutputJSON(json.RawMessage(`[1]`)); len(got) != 1 || got[0].Param != "output" {
		t.Fatalf("not an object: %+v", got)
	}
}

func TestValidateOutputTyped(t *testing.T) {
	// A video spec with its video section left empty: every missing field at
	// once, in document order, then the missing rate choice.
	spec := exampleHLS()
	spec.Video = &Video{Codec: CodecH264}
	spec.Audio.StereoFallback = nil
	var params []string
	for _, e := range ValidateOutput(spec) {
		params = append(params, e.Param)
	}
	want := []string{"output.video.bit_depth", "output.video.color", "output.video.frame_rate.max", "output.video.gop",
		"output.audio.stereo_fallback", "output.renditions.sizes.0.video", "output.renditions.sizes.1.video", "output.video"}
	if !reflect.DeepEqual(params, want) {
		t.Fatalf("params = %v", params)
	}
	// Both a preset and categories in privacy.
	spec = exampleHLS()
	spec.Privacy.Location = "keep"
	if errs := ValidateOutput(spec); len(errs) != 1 || errs[0].Param != "output.privacy.location" {
		t.Fatalf("errors %+v", errs)
	}
}

func TestJobCreateSendsWholeSpec(t *testing.T) {
	f := newFakeAPI(t)
	spec := exampleHLS()
	f.reply(200, `{"id":"job_1","preset":null}`)
	job, err := f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://example.com/a.mp4"), Output: &spec})
	must(t, err)
	if job.Preset != nil {
		t.Fatalf("provenance %+v", job.Preset)
	}
	body := f.last().JSON(t)
	if _, ok := body["preset"]; ok {
		t.Fatalf("preset sent: %v", body)
	}
	b, _ := json.Marshal(body["output"])
	want, _ := json.Marshal(spec)
	var g, w any
	must(t, json.Unmarshal(b, &g))
	must(t, json.Unmarshal(want, &w))
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("output = %s", b)
	}
}

func TestJobCreatePresetOverrides(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"id":"job_1","preset_id":"social-vertical-1080x1920","preset":{"id":"social-vertical-1080x1920@1","version":1,"overrides":{"video":{"frame_rate":{"max":24}}}}}`)
	job, err := f.client().Jobs.Create(context.Background(), &JobCreateParams{
		Input:  URLInput("https://example.com/a.mp4"),
		Preset: String("social-vertical-1080x1920@1"),
		Overrides: OutputOverrides{
			"video":     map[string]any{"frame_rate": map[string]any{"max": 24}},
			"container": map[string]any{"segment_seconds": nil},
		},
	})
	must(t, err)
	if !strings.Contains(string(f.last().Body), `"output":{"container":{"segment_seconds":null},"video":{"frame_rate":{"max":24}}}`) ||
		!strings.Contains(string(f.last().Body), `"preset":"social-vertical-1080x1920@1"`) {
		t.Fatalf("body = %s", f.last().Body)
	}
	if p := job.Preset; p == nil || p.ID != "social-vertical-1080x1920@1" || p.Version != 1 || p.Overrides["video"] == nil {
		t.Fatalf("provenance %+v", job.Preset)
	}

	// A preset alone sends no output.
	f.reply(200, `{"id":"job_2"}`)
	_, err = f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://example.com/a.mp4"), Preset: String("hls-av1-abr")})
	must(t, err)
	if strings.Contains(string(f.last().Body), `"output"`) {
		t.Fatalf("body = %s", f.last().Body)
	}
}

func TestJobCreateRefusedBeforeSending(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	incomplete := NewAudioOutput(ContainerAudio(FormatMP3), Audio{Handling: HandlingEncode, Codec: AudioCodecMP3}, Privacy{})
	spec := exampleHLS()
	cases := []struct {
		name    string
		params  JobCreateParams
		params0 string
		count   int
	}{
		{"incomplete", JobCreateParams{Output: &incomplete}, "output.audio.bitrate", 7},
		{"neither", JobCreateParams{}, "output", 1},
		{"spec with a preset", JobCreateParams{Preset: String("hls-av1-abr"), Output: &spec}, "output", 1},
		{"overrides without a preset", JobCreateParams{Overrides: OutputOverrides{"kind": "video"}}, "output", 1},
	}
	for _, tc := range cases {
		tc.params.Input = URLInput("https://example.com/a.mp4")
		_, err := c.Jobs.Create(context.Background(), &tc.params)
		e, ok := AsError(err)
		if !ok || e.Status != 0 || e.Code != "validation_failed" || !IsInvalidRequest(err) || e.Param != tc.params0 || len(e.Errors) != tc.count {
			t.Fatalf("%s: %v %+v", tc.name, err, e)
		}
	}
	_, err := c.Presets.Create(context.Background(), &PresetCreateParams{Name: "p", Output: &incomplete})
	if e, ok := AsError(err); !ok || len(e.Errors) != 7 {
		t.Fatalf("preset create: %v", err)
	}
	_, err = c.Presets.Replace(context.Background(), "pre_1", &PresetReplaceParams{Name: "p", Output: &incomplete})
	if e, ok := AsError(err); !ok || len(e.Errors) != 7 {
		t.Fatalf("preset replace: %v", err)
	}
	if n := len(f.requests); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
	want := "transcdr: validation_failed: output.audio.bitrate is required"
	_, err = c.Jobs.Create(context.Background(), &JobCreateParams{Output: &incomplete})
	if !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), "(and: output.audio.channels is required") {
		t.Fatalf("message = %s", err)
	}
}

func TestValidationErrorsDecoded(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(422, `{"error":{"type":"invalid_request_error","code":"validation_failed","param":"output.audio.bitrate",
		"message":"output.audio.bitrate is required for audio.codec=aac.",
		"errors":[{"param":"output.audio.bitrate","message":"output.audio.bitrate is required for audio.codec=aac."},
		{"param":"output.privacy","message":"output.privacy is required."}]}}`)
	_, err := f.client().Jobs.Create(context.Background(), &JobCreateParams{Input: URLInput("https://example.com/a.mp4"), Preset: String("x"), Overrides: OutputOverrides{"audio": map[string]any{"codec": "aac"}}})
	e, ok := AsError(err)
	if !ok || e.Status != 422 || len(e.Errors) != 2 || e.Errors[1] != (FieldError{Param: "output.privacy", Message: "output.privacy is required."}) {
		t.Fatalf("%v %+v", err, e)
	}
}

func TestPresetVersions(t *testing.T) {
	f := newFakeAPI(t)
	c := f.client()
	f.reply(200, `{"object":"list","data":[{"object":"preset_version","version":1,"output":`+string(mustJSON(t, exampleHLS()))+`,"created_at":null},
		{"object":"preset_version","version":2,"output":`+string(mustJSON(t, exampleHLS()))+`,"created_at":"2026-09-29T10:00:00Z"}],"has_more":false}`)
	versions, err := c.Presets.Versions(fxCtx, "my preset")
	must(t, err)
	if len(versions) != 2 || versions[0].Version != 1 || versions[0].CreatedAt != nil || versions[1].CreatedAt == nil ||
		versions[1].Output.Kind != KindVideo || !versions[1].Output.Video.Gop.IsSegment() {
		t.Fatalf("versions %+v", versions)
	}
	if r := f.last(); r.Method != "GET" || r.Path != "/v1/presets/my%20preset/versions" {
		t.Fatalf("%s %s", r.Method, r.Path)
	}
	f.reply(200, `{"object":"preset","id":"web-avif","slug":"web-avif","version":3,"output":{"kind":"image"}}`)
	p, err := c.Presets.GetVersion(fxCtx, "web-avif", 3)
	must(t, err)
	if p.Version != 3 || f.last().Path != "/v1/presets/web-avif@3" {
		t.Fatalf("%+v %s", p, f.last().Path)
	}
}

func TestCapabilitiesOutput(t *testing.T) {
	var c Capabilities
	decodeStrict(t, "capabilities.json", &c)
	o := c.Output
	if o == nil || o.Version != 2 || len(o.Kinds) != 3 || len(o.Fields) == 0 || len(o.Groups) != 3 ||
		o.Compatibility.V1Responses.Header != "Transcdr-Output-Spec" || o.Compatibility.V1Responses.Sunset == "" {
		t.Fatalf("output %+v", o)
	}
	// The table the SDK validates with is the one the API lists.
	var paths []string
	for _, f := range o.Fields {
		paths = append(paths, f.Path)
	}
	var mine []string
	for _, f := range outputRules.Fields {
		mine = append(mine, f.Path)
	}
	if !reflect.DeepEqual(paths, mine) {
		t.Fatalf("fields differ:\n api %v\n sdk %v", paths, mine)
	}
	if f := o.Fields[2]; f.Path != "container.format" || !f.Required || f.When[0]["kind"][1] != "audio" {
		t.Fatalf("field %+v", f)
	}
	for _, p := range c.SystemPresets {
		if p.Version < 1 || ValidateOutput(p.Output) != nil {
			t.Fatalf("system preset %s: %+v", p.ID, ValidateOutput(p.Output))
		}
	}
}

func TestResolvedSpecsInFixturesAreComplete(t *testing.T) {
	var j Job
	decodeStrict(t, "job.json", &j)
	jobs := decodeStrictPage[Job](t, "jobs.json")
	presets := decodeStrictPage[Preset](t, "presets.json")
	specs := []OutputSpec{j.Output}
	for _, x := range jobs.Data {
		specs = append(specs, x.Output)
	}
	for _, x := range presets.Data {
		specs = append(specs, x.Output)
	}
	for i, s := range specs {
		if errs := ValidateOutput(s); errs != nil {
			t.Fatalf("spec %d: %+v", i, errs)
		}
		// Re-marshalled, a decoded spec is the JSON it came from.
		var raw any
		must(t, json.Unmarshal(s.Raw(), &raw))
		b, _ := json.Marshal(raw)
		sameJSON(t, s, string(b))
	}
	if j.Preset == nil || j.Preset.ID != *j.PresetID || j.Preset.Version != 1 {
		t.Fatalf("provenance %+v", j.Preset)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	must(t, err)
	return b
}
