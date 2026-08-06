package vidu

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
		return fmt.Sprintf("%s/ent/v2/reference2image", info.ChannelBaseUrl), nil
	default:
		return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	header.Set("Authorization", "Token "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented")
}

type imageRequestPayload struct {
	Model       string   `json:"model"`
	Images      []string `json:"images,omitempty"`
	Prompt      string   `json:"prompt"`
	Seed        int      `json:"seed,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
	Quality     string   `json:"quality,omitempty"`
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	payload := imageRequestPayload{
		Model:       info.UpstreamModelName,
		Prompt:      request.Prompt,
		AspectRatio: request.Size,
		Resolution:  "1K",
	}
	if payload.AspectRatio == "" {
		payload.AspectRatio = "16:9"
	}
	if payload.Model == "" {
		payload.Model = "viduimage-2"
	}

	if request.Quality != "" {
		payload.Quality = request.Quality
	}

	if len(request.ExtraFields) > 0 {
		var extra struct {
			Resolution string `json:"resolution,omitempty"`
			Seed       int    `json:"seed,omitempty"`
		}
		if err := common.Unmarshal(request.ExtraFields, &extra); err == nil {
			if extra.Resolution != "" {
				payload.Resolution = extra.Resolution
			}
			if extra.Seed != 0 {
				payload.Seed = extra.Seed
			}
		}
	}

	return payload, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

type submitResponse struct {
	TaskId string `json:"task_id"`
	State  string `json:"state"`
	Model  string `json:"model"`
}

type taskResultResponse struct {
	State     string     `json:"state"`
	ErrCode   string     `json:"err_code"`
	Creations []creation `json:"creations"`
}

type creation struct {
	URL      string `json:"url"`
	CoverURL string `json:"cover_url"`
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeImagesGenerations {
		usage, err = viduImageHandler(c, resp, info)
	}
	return
}

func viduImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: string(responseBody),
			Type:    "vidu_error",
		}, resp.StatusCode)
	}

	var submitResp submitResponse
	if unmarshalErr := common.Unmarshal(responseBody, &submitResp); unmarshalErr != nil {
		return nil, types.NewOpenAIError(unmarshalErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if submitResp.TaskId == "" {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: "vidu returned empty task_id: " + string(responseBody),
			Type:    "vidu_error",
		}, http.StatusBadGateway)
	}

	taskResult, pollErr := asyncTaskWait(info, submitResp.TaskId)
	if pollErr != nil {
		return nil, types.NewError(pollErr, types.ErrorCodeBadResponse)
	}

	if taskResult.State != "success" {
		errMsg := taskResult.ErrCode
		if errMsg == "" {
			errMsg = "task failed with state: " + taskResult.State
		}
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: errMsg,
			Type:    "vidu_error",
		}, http.StatusBadGateway)
	}

	imageResponse := dto.ImageResponse{
		Created: info.StartTime.Unix(),
	}
	for _, c := range taskResult.Creations {
		if c.URL != "" {
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{
				Url: c.URL,
			})
		}
	}

	jsonResponse, marshalErr := common.Marshal(imageResponse)
	if marshalErr != nil {
		return nil, types.NewError(marshalErr, types.ErrorCodeBadResponseBody)
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return &dto.Usage{}, nil
}

func asyncTaskWait(info *relaycommon.RelayInfo, taskID string) (*taskResultResponse, error) {
	maxAttempts := 60
	interval := 5 * time.Second

	time.Sleep(3 * time.Second)

	for i := 0; i < maxAttempts; i++ {
		result, err := fetchTaskResult(info, taskID)
		if err != nil {
			common.SysLog(fmt.Sprintf("vidu asyncTaskWait fetch err (attempt %d): %s", i, err.Error()))
			time.Sleep(interval)
			continue
		}

		switch result.State {
		case "success":
			return result, nil
		case "failed":
			errMsg := result.ErrCode
			if errMsg == "" {
				errMsg = "task failed"
			}
			return result, nil
		case "created", "queueing", "processing":
			time.Sleep(interval)
			continue
		default:
			return nil, fmt.Errorf("unknown task state: %s", result.State)
		}
	}

	return nil, fmt.Errorf("vidu task timeout after %d attempts", maxAttempts)
}

func fetchTaskResult(info *relaycommon.RelayInfo, taskID string) (*taskResultResponse, error) {
	url := fmt.Sprintf("%s/ent/v2/tasks/%s/creations", info.ChannelBaseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	apiKey := info.ApiKey
	if strings.Contains(apiKey, "|") {
		parts := strings.SplitN(apiKey, "|", 2)
		apiKey = parts[0]
	}
	req.Header.Set("Authorization", "Token "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result taskResultResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal task result failed: %w, body: %s", err, string(body))
	}

	return &result, nil
}

func (a *Adaptor) GetModelList() []string {
	return []string{
		"viduimage-2", "q3-fast", "q2-pro", "q2-fast",
	}
}

func (a *Adaptor) GetChannelName() string {
	return "vidu"
}
