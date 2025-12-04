package config

type TelegramConfig struct {
	AllowedUpdates       []string `env:"ALLOWED_UPDATES" envSeparator:","`
	Token                string   `env:"BOT_TOKEN"`
	WorkersCount         int      `env:"WORKERS_COUNT" envDefault:"1"`
	MessagePerSecond     int      `env:"MESSAGE_PER_SECOND" envDefault:"-1"`
	LocalizationFilePath string   `env:"LOCALIZATION_FILE_PATH"`
}
