package gateway

import (
	"context"
	"fullcycle/chatservice/internal/domains/entity"
)

type ChatGateway interface {
	CreateChat(ctx context.Context, chat, *entity.Chat) error
	FindByChatId(ctx context.Context, chatID string) (*entity.Chat, error)
	SaveChat(ctx context.Context, chat, *entity.Chat) error
}
