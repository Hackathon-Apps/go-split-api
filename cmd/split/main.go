package main

import (
	"flag"
	"github.com/BurntSushi/toml"
	"github.com/Hackathon-Apps/go-split-api/internal/app/config"
	"github.com/Hackathon-Apps/go-split-api/internal/app/split"
	"github.com/Hackathon-Apps/go-split-api/internal/app/storage"
	"github.com/sirupsen/logrus"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"log"
)

var (
	configPath string
)

func init() {
	flag.StringVar(&configPath, "config-path", "configs/split.toml", "path to config file")
}

func main() {
	flag.Parse()

	configuration := config.NewConfiguration()
	if _, err := toml.DecodeFile(configPath, configuration); err != nil {
		log.Fatal(err)
	}

	logger, err := configureLogger(configuration)
	if err != nil {
		log.Fatal(err)
	}

	db, err := storage.Connect(configuration, logger)
	if err != nil {
		log.Fatal(err)
	}

	pool := liteclient.NewConnectionPool()
	api := ton.NewAPIClient(pool)

	server := split.NewServer(configuration, logger, db, api)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}

func configureLogger(cfg *config.Configuration) (*logrus.Logger, error) {
	logger := logrus.New()
	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}

	formatter := &logrus.TextFormatter{}
	formatter.FullTimestamp = true
	formatter.TimestampFormat = "2006-01-02 15:04:05.000"

	logger.SetFormatter(formatter)
	logger.SetLevel(level)
	return logger, nil
}
