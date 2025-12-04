package locale

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"io"
	"os"
)

type LocalizationProvider struct {
	locales *LocalizationFileInfo
}

func NewLocalizationProvider(filePath string) *LocalizationProvider {
	cfgFile, err := os.Open(filePath)

	if err != nil {
		panic(err)
	}

	defer func(cfgFile *os.File) {
		err := cfgFile.Close()
		if err != nil {
			logrus.WithError(err).Error("failed to close file")
		}
	}(cfgFile)

	data, err := io.ReadAll(cfgFile)

	if err != nil {
		panic(err)
	}

	var locales LocalizationFileInfo

	err = json.Unmarshal(data, &locales)

	if err != nil {
		panic(err)
	}

	return &LocalizationProvider{
		locales: &locales,
	}
}

func (l *LocalizationProvider) GetDefaultLocalization(key string) string {
	defaultCulture := l.locales.DefaultCulture

	return l.GetWithCulture(defaultCulture, key)
}

func (l *LocalizationProvider) GetWithCulture(culture, key string) string {
	contentLocalizations, ok := l.locales.LocalizedContent[key]

	if !ok {
		return key
	}

	content, ok := contentLocalizations[culture]

	if !ok {
		content = contentLocalizations[l.locales.DefaultCulture]
	}

	return content
}
