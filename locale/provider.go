package locale

import (
	"encoding/json"
	"fmt"
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

	logrus.WithField("localeKeysAmount", len(locales.LocalizedContent)).Infoln("loaded localization file")

	return &LocalizationProvider{
		locales: &locales,
	}
}

func (l *LocalizationProvider) GetDefaultLocalization(key string, args ...any) string {
	defaultCulture := l.locales.DefaultCulture

	return l.GetWithCulture(defaultCulture, key, args...)
}

func (l *LocalizationProvider) GetWithCulture(culture, key string, args ...any) string {
	contentLocalizations, ok := l.locales.LocalizedContent[key]

	if !ok {
		logrus.Debug("not found content localization by provided key")
		return key
	}

	content, ok := contentLocalizations[culture]

	if !ok {
		logrus.Debug("not found content localization by provided language")
		content, ok = contentLocalizations[l.locales.DefaultCulture]

		logrus.Debug("not found content localization by default language")
		if !ok {
			return key
		}
	}

	if len(args) > 0 {
		return fmt.Sprintf(content, args...)
	}

	return content
}
