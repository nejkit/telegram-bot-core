package utils

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/state"
	"github.com/nejkit/telegram-bot-core/storage"
	"github.com/sirupsen/logrus"
)

type buttonFactoryFunc func(key, value string) tgbotapi.InlineKeyboardButton

func BuildInlineDataKeyboard(
	sortedKeys []string,
	data map[string]string,
	pageSize int,
) *storage.KeyboardInfo {
	return buildInlineKeyboard(
		sortedKeys,
		data,
		pageSize,
		tgbotapi.NewInlineKeyboardButtonData,
	)
}

func BuildInlineURLKeyboard(
	sortedKeys []string,
	data map[string]string,
	pageSize int,
) *storage.KeyboardInfo {
	return buildInlineKeyboard(
		sortedKeys,
		data,
		pageSize,
		tgbotapi.NewInlineKeyboardButtonURL,
	)
}

func buildInlineKeyboard(
	sortedKeys []string,
	data map[string]string,
	pageSize int,
	factoryFunc buttonFactoryFunc,
) *storage.KeyboardInfo {
	if len(sortedKeys) == 0 {
		logrus.Debug("empty data for build inline keyboard")
		return nil
	}

	pagesAmount := (len(sortedKeys) + pageSize - 1) / pageSize

	keyboards := make([]tgbotapi.InlineKeyboardMarkup, pagesAmount)

	for keyIdx := range sortedKeys {
		pageNumber := keyIdx / pageSize

		if keyboards[pageNumber].InlineKeyboard == nil {
			keyboards[pageNumber].InlineKeyboard = make([][]tgbotapi.InlineKeyboardButton, 0)
		}

		buttonTitle := sortedKeys[keyIdx]

		keyboards[pageNumber].InlineKeyboard = append(
			keyboards[pageNumber].InlineKeyboard,
			tgbotapi.NewInlineKeyboardRow(factoryFunc(
				buttonTitle,
				data[buttonTitle],
			),
			),
		)

		if len(keyboards[pageNumber].InlineKeyboard) == pageSize {
			row := tgbotapi.NewInlineKeyboardRow()

			if pageNumber != 0 {
				row = append(row, tgbotapi.NewInlineKeyboardButtonData("Назад", state.WrapCallbackData(
					"set_previous_keyboard",
					"1",
				),
				),
				)
			}

			if pageNumber != pagesAmount {
				row = append(row, tgbotapi.NewInlineKeyboardButtonData("Вперед", state.WrapCallbackData(
					"set_next_keyboard",
					"1",
				),
				),
				)
			}

			if len(row) > 0 {
				keyboards[pageNumber].InlineKeyboard = append(keyboards[pageNumber].InlineKeyboard, row)
			}
		}
	}

	return &storage.KeyboardInfo{Keyboards: keyboards}
}
