package telegram

import (
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/neilkuan/openab-go/acp"
	"github.com/neilkuan/openab-go/config"
	"github.com/neilkuan/openab-go/transcribe"
)

// Adapter implements platform.Platform for Telegram.
type Adapter struct {
	bot     *tgbotapi.BotAPI
	handler *Handler
}

func NewAdapter(cfg config.TelegramConfig, pool *acp.SessionPool, transcriber transcribe.Transcriber) (*Adapter, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	allowed := make(map[int64]bool, len(cfg.AllowedChats))
	for _, id := range cfg.AllowedChats {
		allowed[id] = true
	}

	h := &Handler{
		Bot:             bot,
		Pool:            pool,
		AllowedChats:    allowed,
		ReactionsConfig: cfg.Reactions,
		Transcriber:     transcriber,
	}

	return &Adapter{bot: bot, handler: h}, nil
}

func (a *Adapter) Start() error {
	slog.Info("starting telegram adapter", "bot", a.bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := a.bot.GetUpdatesChan(u)

	go func() {
		for update := range updates {
			a.handler.HandleUpdate(update)
		}
		slog.Debug("telegram update loop exited")
	}()

	return nil
}

func (a *Adapter) Stop() error {
	slog.Info("stopping telegram adapter")
	a.bot.StopReceivingUpdates()
	return nil
}
