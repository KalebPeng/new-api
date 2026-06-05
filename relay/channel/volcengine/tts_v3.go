package volcengine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// VolcengineV3TTSRequest is the request body for the ByteDance V3 HTTP
// unidirectional (HTTP Chunked) TTS endpoint:
// https://openspeech.bytedance.com/api/v3/tts/unidirectional
type VolcengineV3TTSRequest struct {
	User      VolcengineV3User      `json:"user"`
	Namespace string                `json:"namespace,omitempty"`
	ReqParams VolcengineV3ReqParams `json:"req_params"`
}

type VolcengineV3User struct {
	UID string `json:"uid"`
}

type VolcengineV3ReqParams struct {
	Text        string                  `json:"text"`
	Speaker     string                  `json:"speaker"`
	Model       string                  `json:"model,omitempty"`
	SSML        string                  `json:"ssml,omitempty"`
	AudioParams VolcengineV3AudioParams `json:"audio_params"`
	Additions   string                  `json:"additions,omitempty"`
}

type VolcengineV3AudioParams struct {
	Format       string `json:"format,omitempty"`
	SampleRate   int    `json:"sample_rate,omitempty"`
	SpeechRate   *int   `json:"speech_rate,omitempty"`
	LoudnessRate *int   `json:"loudness_rate,omitempty"`
	BitRate      *int   `json:"bit_rate,omitempty"`
	Emotion      string `json:"emotion,omitempty"`
	EmotionScale *int   `json:"emotion_scale,omitempty"`
}

// VolcengineV3TTSResponse is a single JSON frame from the V3 HTTP chunked stream.
type VolcengineV3TTSResponse struct {
	Code     int                   `json:"code"`
	Message  string                `json:"message"`
	Data     string                `json:"data"`
	Sentence json.RawMessage       `json:"sentence,omitempty"`
	Usage    *VolcengineV3TTSUsage `json:"usage,omitempty"`
}

type VolcengineV3TTSUsage struct {
	TextWords int `json:"text_words"`
}

const (
	// v3SuccessFinishCode marks the end-of-synthesis success frame.
	v3SuccessFinishCode = 20000000

	contextKeyV3TTSRequest    = "volcengine_v3_tts_request"
	contextKeyV3TTSResourceID = "volcengine_v3_tts_resource_id"
)

// volcengineV3TTSModels maps the model name (as requested through the gateway)
// to the X-Api-Resource-Id required by the V3 endpoint.
var volcengineV3TTSModels = map[string]string{
	"seed-tts-2.0":         "seed-tts-2.0",
	"seed-tts-1.0":         "seed-tts-1.0",
	"seed-tts-1.0-concurr": "seed-tts-1.0-concurr",
	"seed-icl-2.0":         "seed-icl-2.0",
	"seed-icl-1.0":         "seed-icl-1.0",
	"seed-icl-1.0-concurr": "seed-icl-1.0-concurr",
}

// volcengineV3ResourceID returns the resource id and whether the model is a V3 TTS model.
func volcengineV3ResourceID(model string) (string, bool) {
	resourceID, ok := volcengineV3TTSModels[model]
	return resourceID, ok
}

// isVolcengineV3TTSModel reports whether the given model should use the V3 HTTP path.
func isVolcengineV3TTSModel(model string) bool {
	_, ok := volcengineV3TTSModels[model]
	return ok
}

// volcengineV3DefaultSpeaker is used when the request carries no voice. It is
// the speaker shown in the official V3 single-voice example (doc §2.2) and is
// the only speaker id verified directly from the bundled documentation.
const volcengineV3DefaultSpeaker = "zh_female_shuangkuaisisi_moon_bigtts"

// mapV3Speaker resolves the V3 speaker id from the OpenAI-style voice field.
//
// V3 requires an exact ByteDance speaker id (see the speaker list at
// https://www.volcengine.com/docs/6561/1257544); an unknown/unauthorized
// speaker fails hard with code 45000000. There is no documented 1:1 mapping
// from the OpenAI voice names (alloy/echo/...) to V3 speakers, so we do NOT
// invent one: a non-empty voice is passed through verbatim as the speaker id,
// and an empty voice falls back to the documented default. Callers may also
// override req_params.speaker via request metadata.
func mapV3Speaker(openAIVoice string) string {
	if openAIVoice == "" {
		return volcengineV3DefaultSpeaker
	}
	return openAIVoice
}

// buildV3TTSRequest converts an OpenAI-compatible audio request into the
// ByteDance V3 HTTP unidirectional request body. The optional request.Metadata
// JSON object (if present) is merged on top, allowing callers to override or
// supply fields the OpenAI schema does not expose (e.g. ssml, additions,
// namespace, emotion).
func buildV3TTSRequest(request dto.AudioRequest, encoding string) (VolcengineV3TTSRequest, error) {
	v3Request := VolcengineV3TTSRequest{
		User: VolcengineV3User{
			UID: "openai_relay_user",
		},
		Namespace: "BidirectionalTTS",
		ReqParams: VolcengineV3ReqParams{
			Text:    request.Input,
			Speaker: mapV3Speaker(request.Voice),
			AudioParams: VolcengineV3AudioParams{
				Format:     encoding,
				SampleRate: 24000,
			},
		},
	}

	// OpenAI "speed" (0.25-4.0, default 1.0) maps to ByteDance speech_rate
	// ([-50,100], default 0). speed=1.0 => 0; >1 speeds up, <1 slows down.
	if request.Speed != nil {
		speechRate := openAISpeedToV3SpeechRate(*request.Speed)
		v3Request.ReqParams.AudioParams.SpeechRate = &speechRate
	}

	if len(request.Metadata) > 0 {
		if err := json.Unmarshal(request.Metadata, &v3Request); err != nil {
			return VolcengineV3TTSRequest{}, fmt.Errorf("error unmarshalling metadata to volcengine v3 request: %w", err)
		}
	}

	return v3Request, nil
}

// openAISpeedToV3SpeechRate converts the OpenAI "speed" multiplier into the
// ByteDance speech_rate scale. OpenAI speed is a multiplier centered on 1.0;
// ByteDance speech_rate is a signed integer percentage centered on 0 in the
// range [-50, 100]. We map (speed-1)*100 and clamp to the supported range.
func openAISpeedToV3SpeechRate(speed float64) int {
	rate := int(math.Round((speed - 1.0) * 100))
	if rate < -50 {
		rate = -50
	}
	if rate > 100 {
		rate = 100
	}
	return rate
}

// handleV3TTSResponse consumes the ByteDance V3 HTTP unidirectional (HTTP
// Chunked) response stream. Each chunk is a self-contained JSON frame:
//   - audio frame:      {"code":0,"data":"<base64 audio>"}
//   - text/timestamp:   {"code":0,"data":null,"sentence":{...}}  (skipped)
//   - end-success frame:{"code":20000000,"message":"ok","usage":{"text_words":N}}
//
// Audio frames are base64-decoded and streamed to the client as they arrive.
// A single persistent json.Decoder is used so that frame boundaries split
// across TCP/HTTP chunk reads are handled correctly.
func handleV3TTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, encoding string) (any, *types.NewAPIError) {
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("volcengine v3 tts returned status %d: %s", resp.StatusCode, string(body)),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}

	contentType := getContentTypeByEncoding(encoding)
	c.Header("Content-Type", contentType)
	c.Header("Transfer-Encoding", "chunked")

	decoder := json.NewDecoder(resp.Body)
	textWords := 0
	wroteAudio := false

	for {
		var frame VolcengineV3TTSResponse
		decodeErr := decoder.Decode(&frame)
		if decodeErr == io.EOF {
			break
		}
		if decodeErr != nil {
			// If we have already streamed audio we cannot change the status
			// code; surface the parse failure as a generic error otherwise.
			if wroteAudio {
				break
			}
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to parse volcengine v3 tts frame: %w", decodeErr),
				types.ErrorCodeBadResponseBody,
				http.StatusInternalServerError,
			)
		}

		// End-of-synthesis success frame.
		if frame.Code == v3SuccessFinishCode {
			if frame.Usage != nil {
				textWords = frame.Usage.TextWords
			}
			break
		}

		// Any non-zero, non-success code is an error frame.
		if frame.Code != 0 {
			if wroteAudio {
				break
			}
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("volcengine v3 tts error: code=%d, %s", frame.Code, frame.Message),
				types.ErrorCodeBadResponse,
				http.StatusBadRequest,
			)
		}

		// Skip text/timestamp frames (data is null/empty).
		if frame.Data == "" {
			continue
		}

		audioData, decErr := base64.StdEncoding.DecodeString(frame.Data)
		if decErr != nil {
			if wroteAudio {
				continue
			}
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to decode volcengine v3 audio data: %w", decErr),
				types.ErrorCodeBadResponseBody,
				http.StatusInternalServerError,
			)
		}

		if len(audioData) > 0 {
			if _, writeErr := c.Writer.Write(audioData); writeErr != nil {
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("failed to write audio data: %w", writeErr),
					types.ErrorCodeBadResponse,
					http.StatusInternalServerError,
				)
			}
			c.Writer.Flush()
			wroteAudio = true
		}
	}

	c.Status(http.StatusOK)

	promptTokens := textWords
	if promptTokens == 0 {
		promptTokens = info.GetEstimatePromptTokens()
	}

	return &dto.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: 0,
		TotalTokens:      promptTokens,
	}, nil
}

// marshalV3TTSRequest serialises the V3 request body for transmission.
func marshalV3TTSRequest(v3Request VolcengineV3TTSRequest) (*bytes.Reader, error) {
	jsonData, err := json.Marshal(v3Request)
	if err != nil {
		return nil, fmt.Errorf("error marshalling volcengine v3 request: %w", err)
	}
	return bytes.NewReader(jsonData), nil
}
