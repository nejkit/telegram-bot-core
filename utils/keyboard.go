package utils

import (
	"github.com/go-telegram/bot/models"
	"github.com/nejkit/telegram-bot-core/state"
	"github.com/nejkit/telegram-bot-core/storage"
	"github.com/sirupsen/logrus"
)

type buttonFactoryFunc func(key, value string) models.InlineKeyboardButton

func newInlineKeyboardButtonData(text, data string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: data}
}

func newInlineKeyboardButtonURL(text, url string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, URL: url}
}

func BuildInlineDataKeyboard(
	sortedKeys []string,
	data map[string]string,
	pageSize int,
) *storage.KeyboardInfo {
	return buildInlineKeyboard(
		sortedKeys,
		data,
		pageSize,
		newInlineKeyboardButtonData,
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
		newInlineKeyboardButtonURL,
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

	keyboards := make([]models.InlineKeyboardMarkup, pagesAmount)

	for keyIdx := range sortedKeys {
		pageNumber := keyIdx / pageSize

		if keyboards[pageNumber].InlineKeyboard == nil {
			keyboards[pageNumber].InlineKeyboard = make([][]models.InlineKeyboardButton, 0)
		}

		buttonTitle := sortedKeys[keyIdx]

		keyboards[pageNumber].InlineKeyboard = append(
			keyboards[pageNumber].InlineKeyboard,
			[]models.InlineKeyboardButton{factoryFunc(buttonTitle, data[buttonTitle])},
		)

		if len(keyboards[pageNumber].InlineKeyboard) == pageSize {
			row := make([]models.InlineKeyboardButton, 0, 2)

			if pageNumber != 0 {
				row = append(row, newInlineKeyboardButtonData("Назад", state.WrapCallbackData(
					"set-previous-keyboard",
					"1",
				)))
			}

			if pageNumber+1 != pagesAmount {
				row = append(row, newInlineKeyboardButtonData("Вперед", state.WrapCallbackData(
					"set-next-keyboard",
					"1",
				)))
			}

			if len(row) > 0 {
				keyboards[pageNumber].InlineKeyboard = append(keyboards[pageNumber].InlineKeyboard, row)
			}
		}
	}

	return &storage.KeyboardInfo{Keyboards: keyboards}
}
