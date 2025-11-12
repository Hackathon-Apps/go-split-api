package config

type Configuration struct {
	BindAddress string `toml:"bind_address"`
	LogLevel    string `toml:"log_level"`
	// db
	DbHost string `toml:"db_host"`
	DbPort int    `toml:"db_port"`
	DbName string `toml:"db_name"`
	DbUser string `toml:"db_user"`
	DbPass string `toml:"db_pass"`
	// chain
	SmartContractHex    string `toml:"smart_contract_hex"`
	TonApiToken         string `toml:"ton_api_token"`
	TonCenterApiKey     string `toml:"ton_center_api_key"`
	FeeCollectorAddress string `toml:"fee_collector_address"`
}

func NewConfiguration() *Configuration {
	return &Configuration{
		BindAddress:         ":8081",
		LogLevel:            "debug",
		DbHost:              "localhost",
		DbPort:              5432,
		DbName:              "database",
		DbUser:              "username",
		DbPass:              "password",
		SmartContractHex:    "0xdead",
		TonApiToken:         "token",
		TonCenterApiKey:     "api_key",
		FeeCollectorAddress: "UQ...rW",
	}
}
