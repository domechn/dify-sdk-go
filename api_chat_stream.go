package dify

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type ChatMessageStreamResponse struct {
	Event  string `json:"event"`
	TaskID string `json:"task_id"`
	ID     string `json:"id"`
	Answer string `json:"answer"`
	// Metadata exists when event is "message_end"
	Metadata       ChatMessageStreamMetadataResponse `json:"metadata"`
	CreatedAt      int64                             `json:"created_at"`
	ConversationID string                            `json:"conversation_id"`
}

type ChatMessageStreamMetadataResponse struct {
	Usage struct {
		PromptTokens        int     `json:"prompt_tokens"`
		PromptUnitPrice     string  `json:"prompt_unit_price"`
		PromptPriceUnit     string  `json:"prompt_price_unit"`
		PromptPrice         string  `json:"prompt_price"`
		CompletionTokens    int     `json:"completion_tokens"`
		CompletionUnitPrice string  `json:"completion_unit_price"`
		CompletionPriceUnit string  `json:"completion_price_unit"`
		CompletionPrice     string  `json:"completion_price"`
		TotalTokens         int     `json:"total_tokens"`
		TotalPrice          string  `json:"total_price"`
		Currency            string  `json:"currency"`
		Latency             float64 `json:"latency"`
	} `json:"usage"`
}

type ChatMessageStreamChannelResponse struct {
	ChatMessageStreamResponse
	Err error `json:"-"`
}

func (api *API) ChatMessagesStreamRaw(ctx context.Context, req *ChatMessageRequest) (*http.Response, error) {
	req.ResponseMode = "streaming"

	httpReq, err := api.createBaseRequest(ctx, http.MethodPost, "/v1/chat-messages", req)
	if err != nil {
		return nil, err
	}
	return api.c.sendRequest(httpReq)
}

func (api *API) ChatMessagesStream(ctx context.Context, req *ChatMessageRequest) (chan ChatMessageStreamChannelResponse, error) {
	httpResp, err := api.ChatMessagesStreamRaw(ctx, req)
	if err != nil {
		return nil, err
	}

	streamChannel := make(chan ChatMessageStreamChannelResponse)
	go api.chatMessagesStreamHandle(ctx, httpResp, streamChannel)
	return streamChannel, nil
}

func (api *API) chatMessagesStreamHandle(ctx context.Context, resp *http.Response, streamChannel chan ChatMessageStreamChannelResponse) {
	defer resp.Body.Close()
	defer close(streamChannel)

	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				streamChannel <- ChatMessageStreamChannelResponse{
					Err: fmt.Errorf("error reading line: %w", err),
				}
				return
			}

			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			line = bytes.TrimPrefix(line, []byte("data:"))

			var resp ChatMessageStreamChannelResponse
			if err = json.Unmarshal(line, &resp); err != nil {
				streamChannel <- ChatMessageStreamChannelResponse{
					Err: fmt.Errorf("error unmarshalling event: %w", err),
				}
				return
			} else if resp.Event == "error" {
				streamChannel <- ChatMessageStreamChannelResponse{
					Err: errors.New("error streaming event: " + string(line)),
				}
				return
			} else if resp.Event == "message_end" {
				streamChannel <- resp
				return
			}
			streamChannel <- resp
		}
	}
}
