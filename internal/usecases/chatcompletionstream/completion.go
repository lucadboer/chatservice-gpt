package chatservicegpt

import (
	"context"
	"errors"
	"fullcycle/chatservice/internal/domains/entity"
	"fullcycle/chatservice/internal/domains/gateway"
	"io"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type ChatCompletionConfigInputDTO struct {
	Model                string
	ModelMaxTokens       int
	Temperature          float32  // 0.0 to 1.0
	TopP                 float32  // 0.0 to 1.0 - to a low value, like 0.1, the model will be very conservative in its word choices, and will tend to generate relatively predictable prompts
	N                    int      // number of messages to generate
	Stop                 []string // list of tokens to stop on
	MaxTokens            int      // number of tokens to generate
	PresencePenalty      float32  // -2.0 to 2.0 - Number between -2.0 and 2.0. Positive values penalize new tokens based on whether they appear in the text so far, increasing the model's likelihood to talk about new topics.
	FrequencyPenalty     float32  // -2.0 to 2.0 - Number between -2.0 and 2.0. Positive values penalize new tokens based on their existing frequency in the text so far, increasing the model's likelihood to talk about new topics.
	InitialSystemMessage string
}

type ChatCompletionInputDTO struct {
	ChatID  string
	UserID  string
	Message string
	Config  ChatCompletionConfigInputDTO
}

type ChatCompletionOutputDTO struct {
	ChatID  string
	UserID  string
	Content string
}

type ChatCompletionUseCase struct {
	chatGateway  gateway.ChatGateway
	OpenAiCLient *openai.Client
	Stream       chan ChatCompletionOutputDTO
}

func NewChatCompletionUseCase(chatGateway gateway.ChatGateway, openaiClient *openai.Client, stream chan ChatCompletionOutputDTO) *ChatCompletionUseCase {
	return &ChatCompletionUseCase{
		chatGateway:  chatGateway,
		OpenAiCLient: openaiClient,
		Stream:       stream,
	}
}

func (uc *ChatCompletionUseCase) Execute(ctx context.Context, input ChatCompletionInputDTO) (*ChatCompletionOutputDTO, error) {
	chat, err := uc.chatGateway.FindByChatId(ctx, input.ChatID)

	if err != nil {
		if err.Error() == "chat not found" {
			chat, err = CreateNewChat(input)

			if err != nil {
				return nil, errors.New("error creating chat: " + err.Error())
			}

			err = uc.chatGateway.CreateChat(ctx, chat)

			if err != nil {
				return nil, errors.New("error persisting new chat: " + err.Error())
			}
		} else {
			return nil, errors.New("error fetching existing chat: " + err.Error())
		}
	}
	userMessage, err := entity.NewMessage("user", input.Message, chat.Config.Model)
	err = chat.AddMessage(userMessage)

	if err != nil {
		return nil, errors.New("error adding message: " + err.Error())
	}

	if err != nil {
		return nil, errors.New("error creating message: " + err.Error())
	}

	messages := []openai.ChatCompletionMessage{}
	for _, msg := range chat.Messages {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	resp, err := uc.OpenAiCLient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:            chat.Config.Model.Name,
			Messages:         messages,
			MaxTokens:        chat.Config.MaxTokens,
			Temperature:      chat.Config.Temperature,
			TopP:             chat.Config.TopP,
			PresencePenalty:  chat.Config.PresencePenalty,
			FrequencyPenalty: chat.Config.FrequencyPenalty,
			Stop:             chat.Config.Stop,
		},
	)
	if err != nil {
		return nil, errors.New("error openai: " + err.Error())
	}

	var fullResponses strings.Builder
	for {
		response, err := resp.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("error streaming response" + err.Error())
		}
		fullResponses.WriteString(response.Choices[0].Delta.Content)
		r := ChatCompletionOutputDTO{
			ChatID:  chat.ID,
			UserID:  input.UserID,
			Content: fullResponses.String(),
		}
		uc.Stream <- r
	}
	assistant, err := entity.NewMessage("assistant", fullResponses.String(), chat.Config.Model)
	if err != nil {
		return nil, errors.New("error creating new message" + err.Error())
	}

	err = chat.AddMessage(assistant)
	if err != nil {
		return nil, errors.New("error adding new message" + err.Error())
	}

	err = uc.chatGateway.SaveChat(ctx, chat)
	if err != nil {
		return nil, errors.New("error saving chat" + err.Error())
	}

	return &ChatCompletionOutputDTO{
		ChatID:  chat.ID,
		UserID:  input.UserID,
		Content: fullResponses.String(),
	}, nil
}

func CreateNewChat(input ChatCompletionInputDTO) (*entity.Chat, error) {
	model := entity.NewModel(input.Config.Model, input.Config.ModelMaxTokens)
	chatConfig := &entity.ChatConfig{
		Temperature:      input.Config.Temperature,
		TopP:             input.Config.TopP,
		N:                input.Config.N,
		Stop:             input.Config.Stop,
		MaxTokens:        input.Config.MaxTokens,
		PresencePenalty:  input.Config.PresencePenalty,
		FrequencyPenalty: input.Config.FrequencyPenalty,
		Model:            model,
	}

	initialMessage, err := entity.NewMessage("system", input.Config.InitialSystemMessage, model)

	if err != nil {
		return nil, errors.New("error creating initial message" + err.Error())
	}
	chat, err := entity.NewChat(input.UserID, initialMessage, chatConfig)

	if err != nil {
		return nil, errors.New("error creating new chat" + err.Error())
	}
	return chat, nil
}
