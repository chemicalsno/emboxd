package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type letterboxd struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	LogFilms bool   `yaml:"log_films"`
}

type emby struct {
	Username string `yaml:"username"`
}

type plex struct {
	Username string `yaml:"username"`
	ID       string `yaml:"id"`
}

type user struct {
	Letterboxd letterboxd `yaml:"letterboxd"`
	Emby       emby       `yaml:"emby"`
	Plex       plex       `yaml:"plex"`
}

type Config struct {
	Users []user `yaml:"users"`
}

func Load(filename string) Config {
	var data, readErr = os.ReadFile(filename)
	if readErr != nil {
		panic(readErr)
	}

	var config Config
	if yamlErr := yaml.Unmarshal(data, &config); yamlErr != nil {
		panic(yamlErr)
	}
	return config
}
