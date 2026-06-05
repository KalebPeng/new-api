package volcengine

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestIsVolcengineV3TTSModel(t *testing.T) {
	t.Parallel()

	v3Models := []string{
		"seed-tts-2.0", "seed-tts-1.0", "seed-tts-1.0-concurr",
		"seed-icl-2.0", "seed-icl-1.0", "seed-icl-1.0-concurr",
	}
	for _, m := range v3Models {
		if !isVolcengineV3TTSModel(m) {
			t.Errorf("isVolcengineV3TTSModel(%q) = false, want true", m)
		}
		if rid, ok := volcengineV3ResourceID(m); !ok || rid != m {
			t.Errorf("volcengineV3ResourceID(%q) = (%q, %v), want (%q, true)", m, rid, ok, m)
		}
	}

	notV3 := []string{"", "Doubao-pro-32k", "tts-1", "seed-tts", "seed-1-6-thinking-250715"}
	for _, m := range notV3 {
		if isVolcengineV3TTSModel(m) {
			t.Errorf("isVolcengineV3TTSModel(%q) = true, want false", m)
		}
	}
}

func TestMapV3Speaker(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		// Empty voice falls back to the only doc-verified speaker.
		"": volcengineV3DefaultSpeaker,
		// Any non-empty voice is passed through verbatim as the V3 speaker id.
		"zh_female_shuangkuaisisi_moon_bigtts": "zh_female_shuangkuaisisi_moon_bigtts",
		"nova":                                 "nova",
		"my_custom_speaker_v3":                 "my_custom_speaker_v3",
	}
	for voice, want := range cases {
		if got := mapV3Speaker(voice); got != want {
			t.Errorf("mapV3Speaker(%q) = %q, want %q", voice, got, want)
		}
	}
}

func TestOpenAISpeedToV3SpeechRate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		speed float64
		want  int
	}{
		{1.0, 0},
		{1.5, 50},
		{0.5, -50},
		{2.0, 100},
		{4.0, 100},  // clamp high
		{0.25, -50}, // clamp low
		{0.1, -50},  // clamp low
	}
	for _, tc := range cases {
		if got := openAISpeedToV3SpeechRate(tc.speed); got != tc.want {
			t.Errorf("openAISpeedToV3SpeechRate(%v) = %d, want %d", tc.speed, got, tc.want)
		}
	}
}

func TestBuildV3TTSRequest(t *testing.T) {
	t.Parallel()

	speed := 1.2
	request := dto.AudioRequest{
		Model:          "seed-tts-2.0",
		Input:          "你好，世界",
		Voice:          "zh_female_shuangkuaisisi_moon_bigtts",
		ResponseFormat: "opus",
		Speed:          &speed,
	}

	v3Request, err := buildV3TTSRequest(request, mapEncoding(request.ResponseFormat))
	if err != nil {
		t.Fatalf("buildV3TTSRequest returned error: %v", err)
	}

	if v3Request.ReqParams.Text != "你好，世界" {
		t.Errorf("text = %q, want %q", v3Request.ReqParams.Text, "你好，世界")
	}
	if v3Request.ReqParams.Speaker != "zh_female_shuangkuaisisi_moon_bigtts" {
		t.Errorf("speaker = %q, want passthrough speaker", v3Request.ReqParams.Speaker)
	}
	if v3Request.ReqParams.AudioParams.Format != "ogg_opus" {
		t.Errorf("format = %q, want %q", v3Request.ReqParams.AudioParams.Format, "ogg_opus")
	}
	if v3Request.ReqParams.AudioParams.SampleRate != 24000 {
		t.Errorf("sample_rate = %d, want 24000", v3Request.ReqParams.AudioParams.SampleRate)
	}
	if v3Request.ReqParams.AudioParams.SpeechRate == nil || *v3Request.ReqParams.AudioParams.SpeechRate != 20 {
		t.Errorf("speech_rate = %v, want 20", v3Request.ReqParams.AudioParams.SpeechRate)
	}
	if v3Request.Namespace != "BidirectionalTTS" {
		t.Errorf("namespace = %q, want %q", v3Request.Namespace, "BidirectionalTTS")
	}
}

func TestBuildV3TTSRequestMetadataOverride(t *testing.T) {
	t.Parallel()

	request := dto.AudioRequest{
		Model:    "seed-tts-2.0",
		Input:    "hello",
		Voice:    "nova",
		Metadata: json.RawMessage(`{"req_params":{"speaker":"custom_speaker","additions":"{\"k\":1}"}}`),
	}

	v3Request, err := buildV3TTSRequest(request, "mp3")
	if err != nil {
		t.Fatalf("buildV3TTSRequest returned error: %v", err)
	}
	if v3Request.ReqParams.Speaker != "custom_speaker" {
		t.Errorf("speaker = %q, want override %q", v3Request.ReqParams.Speaker, "custom_speaker")
	}
	if v3Request.ReqParams.Additions != `{"k":1}` {
		t.Errorf("additions = %q, want override", v3Request.ReqParams.Additions)
	}
}

type v3NopReadCloser struct {
	*strings.Reader
}

func (v3NopReadCloser) Close() error { return nil }

func TestHandleV3TTSResponse(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	audioChunk := base64.StdEncoding.EncodeToString([]byte("AUDIOBYTES"))
	// Two audio frames, a skipped text frame, then the success end frame.
	frames := strings.Join([]string{
		`{"code":0,"message":"","data":"` + audioChunk + `"}`,
		`{"code":0,"data":null,"sentence":{"text":"hi"}}`,
		`{"code":0,"message":"","data":"` + audioChunk + `"}`,
		`{"code":20000000,"message":"ok","data":null,"usage":{"text_words":7}}`,
	}, "")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       v3NopReadCloser{Reader: strings.NewReader(frames)},
	}

	usage, apiErr := handleV3TTSResponse(c, resp, info, "mp3")
	if apiErr != nil {
		t.Fatalf("handleV3TTSResponse returned error: %v", apiErr)
	}
	u, ok := usage.(*dto.Usage)
	if !ok || u == nil {
		t.Fatalf("usage = %#v, want *dto.Usage", usage)
	}
	if u.PromptTokens != 7 || u.TotalTokens != 7 {
		t.Errorf("usage tokens = %d/%d, want 7/7 from text_words", u.PromptTokens, u.TotalTokens)
	}

	if got := recorder.Body.String(); got != "AUDIOBYTESAUDIOBYTES" {
		t.Errorf("audio body = %q, want two concatenated decoded chunks", got)
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("content-type = %q, want audio/mpeg", ct)
	}
}

func TestHandleV3TTSResponseErrorFrame(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       v3NopReadCloser{Reader: strings.NewReader(`{"code":40000001,"message":"invalid speaker","data":null}`)},
	}

	usage, apiErr := handleV3TTSResponse(c, resp, info, "mp3")
	if apiErr == nil {
		t.Fatalf("handleV3TTSResponse returned nil error for error frame, usage=%#v", usage)
	}
}
