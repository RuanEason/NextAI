package config

import (
	"os"
)

type Config struct {
	Host    string
	Port    string
	DataDir string
	APIKey  string
	WebDir  string
}

func Load() Config {
	host := os.Getenv("NEXTAI_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("NEXTAI_PORT")
	if port == "" {
		port = "8088"
	}
	dataDir := os.Getenv("NEXTAI_DATA_DIR")
	if dataDir == "" {
		dataDir = ".data"
	}
	apiKey := os.Getenv("NEXTAI_API_KEY")
	webDir := os.Getenv("NEXTAI_WEB_DIR")
	return Config{Host: host, Port: port, DataDir: dataDir, APIKey: apiKey, WebDir: webDir}
}
