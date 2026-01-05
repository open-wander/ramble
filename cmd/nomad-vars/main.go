package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database struct {
		DBName   string `yaml:"dbname"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
	} `yaml:"database"`
	Server struct {
		SessionSecret string `yaml:"SessionSecret"`
		BaseURL       string `yaml:"BaseURL"`
		AutoSeed      string `yaml:"AutoSeed"`
	} `yaml:"server"`
	Seed struct {
		Username string `yaml:"InitialUserUsername"`
		Email    string `yaml:"InitialUserEmail"`
		Password string `yaml:"InitialUserPassword"`
	} `yaml:"seed"`
	Email struct {
		SMTPHost     string `yaml:"smtpHost"`
		SMTPPort     string `yaml:"smtpPort"`
		SMTPUser     string `yaml:"smtpUser"`
		SMTPPassword string `yaml:"smtpPassword"`
		FromAddress  string `yaml:"fromAddress"`
	} `yaml:"email"`
	OAuth struct {
		GithubKey       string `yaml:"githubKey"`
		GithubSecret    string `yaml:"githubSecret"`
		GitlabKey       string `yaml:"gitlabKey"`
		GitlabSecret    string `yaml:"gitlabSecret"`
	} `yaml:"oauth"`
}

func main() {
	// Read config.yml
	data, err := ioutil.ReadFile("config.yml")
	if err != nil {
		log.Fatalf("Failed to read config.yml: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse config.yml: %v", err)
	}

	// Prepare arguments for nomad var put
	args := []string{"var", "put", "nomad/jobs/rmbl"}
	
	// Database
	args = append(args, fmt.Sprintf("db_name=%s", cfg.Database.DBName))
	args = append(args, fmt.Sprintf("db_user=%s", cfg.Database.Username))
	args = append(args, fmt.Sprintf("db_password=%s", cfg.Database.Password))
	args = append(args, fmt.Sprintf("db_host=%s", cfg.Database.Host))
	args = append(args, fmt.Sprintf("db_port=%s", cfg.Database.Port))
	
	// Server
	args = append(args, fmt.Sprintf("session_secret=%s", cfg.Server.SessionSecret))
	args = append(args, fmt.Sprintf("base_url=%s", cfg.Server.BaseURL))
	args = append(args, fmt.Sprintf("auto_seed=%s", cfg.Server.AutoSeed))
	
	// Seed
	args = append(args, fmt.Sprintf("initial_user_username=%s", cfg.Seed.Username))
	args = append(args, fmt.Sprintf("initial_user_email=%s", cfg.Seed.Email))
	args = append(args, fmt.Sprintf("initial_user_password=%s", cfg.Seed.Password))

	// Email
	args = append(args, fmt.Sprintf("smtp_host=%s", cfg.Email.SMTPHost))
	args = append(args, fmt.Sprintf("smtp_port=%s", cfg.Email.SMTPPort))
	args = append(args, fmt.Sprintf("smtp_user=%s", cfg.Email.SMTPUser))
	args = append(args, fmt.Sprintf("smtp_password=%s", cfg.Email.SMTPPassword))
	args = append(args, fmt.Sprintf("from_address=%s", cfg.Email.FromAddress))

	// OAuth
	args = append(args, fmt.Sprintf("github_key=%s", cfg.OAuth.GithubKey))
	args = append(args, fmt.Sprintf("github_secret=%s", cfg.OAuth.GithubSecret))
	args = append(args, fmt.Sprintf("gitlab_key=%s", cfg.OAuth.GitlabKey))
	args = append(args, fmt.Sprintf("gitlab_secret=%s", cfg.OAuth.GitlabSecret))

	cmd := exec.Command("nomad", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	fmt.Printf("Executing: nomad %v (hiding values for security)\n", args[0:3])
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to set nomad vars: %v", err)
	}
}
