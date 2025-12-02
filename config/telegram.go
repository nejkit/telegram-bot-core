package config

type TelegramConfig struct {
	AllowedUpdates []string
	Token          string
	WorkersCount   int
}
