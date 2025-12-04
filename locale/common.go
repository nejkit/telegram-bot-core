package locale

type LocalizationFileInfo struct {
	DefaultCulture   string                       `json:"defaultCulture"`
	LocalizedContent map[string]map[string]string `json:"localizedContent"`
}
